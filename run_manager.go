package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type RunManager struct {
	cfg    Config
	logger *log.Logger
	client *UpstreamClient

	mu    sync.RWMutex
	pools []*tokenPool
	next  atomic.Uint64

	stopCh chan struct{}
	wg     sync.WaitGroup
}

type tokenPool struct {
	name   string
	token  string
	userID string // fetched from /api/v1/me at startup
	cfg    Config
	client *UpstreamClient
	logger *log.Logger

	mu               sync.Mutex
	runs             map[string]*managedRun // agentID -> current run
	draining         []*managedRun
	session          *cachedSession
	sessionRefreshCh chan struct{}
	lastError        string
	cooldownUntil    time.Time
}

type managedRun struct {
	id           string
	agentID      string
	model        string
	startedAt    time.Time
	inflight     int
	requestCount int
	finishing    bool
	// gemini runs execute as a child of a base2-free-deepseek-flash parent run;
	// id is the child (used for chat), parentRunID the root to finalize after it.
	parentRunID string
}

type runLease struct {
	pool *tokenPool
	run  *managedRun
}

type tokenSnapshot struct {
	Name              string        `json:"name"`
	Runs              []runSnapshot `json:"runs"`
	DrainingRuns      int           `json:"draining_runs"`
	SessionStatus     string        `json:"session_status,omitempty"`
	SessionInstanceID string        `json:"session_instance_id,omitempty"`
	SessionExpiresAt  time.Time     `json:"session_expires_at,omitempty"`
	SessionPosition   int           `json:"session_position,omitempty"`
	SessionQueueDepth int           `json:"session_queue_depth,omitempty"`
	SessionPollAt     time.Time     `json:"session_poll_at,omitempty"`
	CooldownUntil     time.Time     `json:"cooldown_until,omitempty"`
	LastError         string        `json:"last_error,omitempty"`
}

type runSnapshot struct {
	AgentID      string    `json:"agent_id"`
	RunID        string    `json:"run_id"`
	StartedAt    time.Time `json:"started_at"`
	Inflight     int       `json:"inflight"`
	RequestCount int       `json:"request_count"`
}

type waitingRoomError struct {
	Token      string
	Position   int
	QueueDepth int
	RetryAfter time.Duration
}

func (e *waitingRoomError) Error() string {
	if e == nil {
		return "freebuff waiting room queued"
	}

	message := "freebuff waiting room queued"
	if e.Token != "" {
		message += " for " + e.Token
	}
	if e.Position > 0 {
		if e.QueueDepth >= e.Position {
			message += fmt.Sprintf(" (position %d/%d)", e.Position, e.QueueDepth)
		} else {
			message += fmt.Sprintf(" (position %d)", e.Position)
		}
	}
	if e.RetryAfter > 0 {
		message += fmt.Sprintf(", retry in about %s", e.RetryAfter.Round(time.Second))
	}
	return message
}

func NewRunManager(cfg Config, client *UpstreamClient, logger *log.Logger) *RunManager {
	pools := make([]*tokenPool, 0, len(cfg.AuthTokens))
	for index, token := range cfg.AuthTokens {
		pools = append(pools, &tokenPool{
			name:   fmt.Sprintf("token-%d", index+1),
			token:  token,
			cfg:    cfg,
			client: client,
			runs:   make(map[string]*managedRun),
			logger: logger,
		})
	}

	return &RunManager{
		cfg:    cfg,
		logger: logger,
		client: client,
		pools:  pools,
		stopCh: make(chan struct{}),
	}
}

func (m *RunManager) Start(ctx context.Context) {
	// Fetch user IDs for x-freebuff-acting-user-id header (gates free-mode chat).
	m.mu.RLock()
	pools := m.pools
	m.mu.RUnlock()
	for _, pool := range pools {
		uid, err := pool.client.FetchUserID(ctx, pool.token)
		if err != nil {
			m.logger.Printf("%s: fetch user ID failed (chat may be gated): %v", pool.name, err)
		} else {
			pool.userID = uid
			m.logger.Printf("%s: acting-user-id=%s", pool.name, uid)
		}
	}

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				maintainCtx, cancel := context.WithTimeout(context.Background(), m.cfg.RequestTimeout)
				m.mu.RLock()
				snap := m.pools
				m.mu.RUnlock()
				for _, pool := range snap {
					if err := pool.maintain(maintainCtx); err != nil {
						m.logger.Printf("%s: maintenance failed: %v", pool.name, err)
					}
				}
				cancel()
			case <-m.stopCh:
				return
			}
		}
	}()
}

func (m *RunManager) Close(ctx context.Context) {
	close(m.stopCh)
	m.wg.Wait()
	m.mu.RLock()
	pools := m.pools
	m.mu.RUnlock()
	for _, pool := range pools {
		if err := pool.shutdown(ctx); err != nil {
			m.logger.Printf("%s: shutdown failed: %v", pool.name, err)
		}
	}
}

func (m *RunManager) Acquire(ctx context.Context, agentID, model string) (*runLease, error) {
	m.mu.RLock()
	pools := m.pools
	m.mu.RUnlock()

	if len(pools) == 0 {
		return nil, errors.New("no auth tokens configured")
	}

	startIndex := int(m.next.Add(1)-1) % len(pools)
	var errs []string
	var waiting []*waitingRoomError
	for offset := 0; offset < len(pools); offset++ {
		pool := pools[(startIndex+offset)%len(pools)]
		lease, err := pool.acquire(ctx, agentID, model)
		if err == nil {
			return lease, nil
		}
		var waitingErr *waitingRoomError
		if errors.As(err, &waitingErr) {
			waiting = append(waiting, waitingErr)
		}
		errs = append(errs, fmt.Sprintf("%s: %v", pool.name, err))
	}

	if len(waiting) == len(pools) && len(waiting) > 0 {
		best := waiting[0]
		for _, candidate := range waiting[1:] {
			if candidate != nil && (best == nil || (candidate.Position > 0 && candidate.Position < best.Position)) {
				best = candidate
			}
		}
		if best != nil {
			return nil, best
		}
	}

	return nil, fmt.Errorf("unable to acquire run from any token (%s)", strings.Join(errs, "; "))
}

func (m *RunManager) Release(lease *runLease) {
	if lease == nil || lease.pool == nil || lease.run == nil {
		return
	}
	lease.pool.release(lease.run)
}

func (m *RunManager) Invalidate(lease *runLease, reason string) {
	if lease == nil || lease.pool == nil || lease.run == nil {
		return
	}
	lease.pool.invalidate(lease.run, reason)
}

func (m *RunManager) Cooldown(lease *runLease, duration time.Duration, reason string) {
	if lease == nil || lease.pool == nil {
		return
	}
	lease.pool.markCooldown(duration, reason)
}

func (m *RunManager) Snapshots() []tokenSnapshot {
	m.mu.RLock()
	pools := m.pools
	m.mu.RUnlock()
	snapshots := make([]tokenSnapshot, 0, len(pools))
	for _, pool := range pools {
		snapshots = append(snapshots, pool.snapshot())
	}
	return snapshots
}

func (p *tokenPool) acquire(ctx context.Context, agentID, model string) (*runLease, error) {
	p.mu.Lock()
	if now := time.Now(); now.Before(p.cooldownUntil) {
		cooldownUntil := p.cooldownUntil
		p.mu.Unlock()
		return nil, fmt.Errorf("token cooling down until %s", cooldownUntil.Format(time.RFC3339))
	}
	run := p.runs[agentID]
	needsRotate := run == nil || time.Since(run.startedAt) >= p.cfg.RotationInterval
	p.mu.Unlock()

	if needsRotate {
		if err := p.rotateAgent(ctx, agentID, model); err != nil {
			return nil, err
		}
	}

	if _, _, err := p.ensureSession(ctx, model); err != nil {
		return nil, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	run = p.runs[agentID]
	if run == nil {
		return nil, errors.New("run missing after rotation")
	}
	run.inflight++
	run.requestCount++
	return &runLease{pool: p, run: run}, nil
}

func (p *tokenPool) maintain(ctx context.Context) error {
	p.mu.Lock()
	var toRotate []struct{ agentID, model string }
	for agentID, run := range p.runs {
		if time.Since(run.startedAt) >= p.cfg.RotationInterval {
			toRotate = append(toRotate, struct{ agentID, model string }{agentID, run.model})
		}
	}
	draining := append([]*managedRun(nil), p.draining...)
	p.mu.Unlock()

	for _, target := range toRotate {
		if err := p.rotateAgent(ctx, target.agentID, target.model); err != nil {
			p.logger.Printf("%s: rotate agent %s failed: %v", p.name, target.agentID, err)
		}
	}

	if _, _, err := p.ensureSession(ctx, ""); err != nil {
		p.logger.Printf("%s: refresh free session failed: %v", p.name, err)
	}

	for _, run := range draining {
		if err := p.finishIfReady(run); err != nil {
			p.logger.Printf("%s: finish draining run %s failed: %v", p.name, run.id, err)
		}
	}
	return nil
}

func (p *tokenPool) shutdown(ctx context.Context) error {
	p.mu.Lock()
	var allRuns []*managedRun
	for _, run := range p.runs {
		allRuns = append(allRuns, run)
	}
	allRuns = append(allRuns, p.draining...)
	p.runs = make(map[string]*managedRun)
	p.draining = nil
	p.mu.Unlock()

	var errs []string
	for _, run := range allRuns {
		if err := p.client.FinishRun(ctx, p.token, p.userID, run.id, run.requestCount); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if err := p.endSession(ctx); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// rotateAgent starts a fresh run chain for the agent, mirroring the CLI:
//   - normal agents: root run + a context-pruner child run that is recorded and
//     finished immediately, then step 1 on the root referencing the child.
//   - gemini agents: a base2-free-deepseek-flash parent run with the gemini
//     subagent as the chat child; chat uses the child run id.
func (p *tokenPool) rotateAgent(ctx context.Context, agentID, model string) error {
	p.mu.Lock()
	if now := time.Now(); now.Before(p.cooldownUntil) {
		cooldownUntil := p.cooldownUntil
		p.mu.Unlock()
		return fmt.Errorf("token cooling down until %s", cooldownUntil.Format(time.RFC3339))
	}
	p.mu.Unlock()

	newRun, err := p.startRunChain(ctx, agentID, model)
	if err != nil {
		p.mu.Lock()
		p.lastError = err.Error()
		p.mu.Unlock()
		return err
	}

	p.mu.Lock()
	oldRun := p.runs[agentID]
	p.runs[agentID] = newRun
	p.lastError = ""
	if oldRun != nil {
		p.draining = append(p.draining, oldRun)
	}
	p.mu.Unlock()

	if oldRun != nil {
		go func(run *managedRun) {
			if err := p.finishIfReady(run); err != nil {
				p.logger.Printf("%s: finish rotated run %s (agent %s) failed: %v", p.name, run.id, run.agentID, err)
			}
		}(oldRun)
	}
	return nil
}

func (p *tokenPool) startRunChain(ctx context.Context, agentID, model string) (*managedRun, error) {
	startedAt := time.Now().UTC().Format(time.RFC3339)

	if isGeminiModel(model) {
		parentRunID, err := p.client.StartRun(ctx, p.token, p.userID, geminiParentAgentID, nil)
		if err != nil {
			return nil, err
		}
		chatRunID, err := p.client.StartRun(ctx, p.token, p.userID, geminiChatAgentForModel(model), []string{parentRunID})
		if err != nil {
			_ = p.client.FinishRun(ctx, p.token, p.userID, parentRunID, 0)
			return nil, err
		}
		return &managedRun{
			id:          chatRunID,
			parentRunID: parentRunID,
			agentID:     agentID,
			model:       model,
			startedAt:   time.Now(),
		}, nil
	}

	runID, err := p.client.StartRun(ctx, p.token, p.userID, agentID, nil)
	if err != nil {
		return nil, err
	}

	childRunID, err := p.client.StartRun(ctx, p.token, p.userID, contextPrunerAgentID, []string{runID})
	if err != nil {
		// Pruner child is bookkeeping only; a failure here must not sink the chat.
		p.logger.Printf("%s: start context-pruner child failed (continuing): %v", p.name, err)
		return &managedRun{id: runID, agentID: agentID, model: model, startedAt: time.Now()}, nil
	}
	childStartedAt := time.Now().UTC().Format(time.RFC3339)
	if err := p.client.RecordRunStep(ctx, p.token, p.userID, childRunID, 1, nil, nil, childStartedAt); err != nil {
		p.logger.Printf("%s: record pruner step failed (continuing): %v", p.name, err)
	}
	if err := p.client.FinishRun(ctx, p.token, p.userID, childRunID, 2); err != nil {
		p.logger.Printf("%s: finish pruner run failed (continuing): %v", p.name, err)
	}
	if err := p.client.RecordRunStep(ctx, p.token, p.userID, runID, 1, []string{childRunID}, nil, startedAt); err != nil {
		p.logger.Printf("%s: record root step 1 failed (continuing): %v", p.name, err)
	}

	return &managedRun{id: runID, agentID: agentID, model: model, startedAt: time.Now()}, nil
}


func (p *tokenPool) release(run *managedRun) {
	if run == nil {
		return
	}

	p.mu.Lock()
	if run.inflight > 0 {
		run.inflight--
	}
	p.mu.Unlock()

	if err := p.finishIfReady(run); err != nil {
		p.logger.Printf("%s: finish released run %s failed: %v", p.name, run.id, err)
	}
}

func (p *tokenPool) finishIfReady(run *managedRun) error {
	p.mu.Lock()
	if run == nil || run.inflight > 0 || run.finishing {
		p.mu.Unlock()
		return nil
	}
	// Only finish if this run is no longer the current run for its agent
	if current, ok := p.runs[run.agentID]; ok && current == run {
		p.mu.Unlock()
		return nil
	}
	run.finishing = true
	p.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.RequestTimeout)
	defer cancel()

	if err := p.client.FinishRun(ctx, p.token, p.userID, run.id, run.requestCount); err != nil {
		p.mu.Lock()
		run.finishing = false
		p.lastError = err.Error()
		p.mu.Unlock()
		return err
	}
	if run.parentRunID != "" {
		if err := p.client.FinishRun(ctx, p.token, p.userID, run.parentRunID, run.requestCount); err != nil {
			p.logger.Printf("%s: finish gemini parent run %s failed: %v", p.name, run.parentRunID, err)
		}
	}

	p.mu.Lock()
	filtered := p.draining[:0]
	for _, drainingRun := range p.draining {
		if drainingRun != run {
			filtered = append(filtered, drainingRun)
		}
	}
	p.draining = filtered
	p.mu.Unlock()
	return nil
}

func (p *tokenPool) invalidate(run *managedRun, reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Remove from current runs if it matches
	if current, ok := p.runs[run.agentID]; ok && current == run {
		delete(p.runs, run.agentID)
	}

	filtered := p.draining[:0]
	for _, drainingRun := range p.draining {
		if drainingRun != run {
			filtered = append(filtered, drainingRun)
		}
	}
	p.draining = filtered
	if reason != "" {
		p.lastError = reason
	}
}

func (p *tokenPool) markCooldown(duration time.Duration, reason string) {
	if duration <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cooldownUntil = time.Now().Add(duration)
	if reason != "" {
		p.lastError = reason
	}
}

func (p *tokenPool) snapshot() tokenSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()

	snapshot := tokenSnapshot{
		Name:          p.name,
		DrainingRuns:  len(p.draining),
		CooldownUntil: p.cooldownUntil,
		LastError:     p.lastError,
	}
	if p.session != nil {
		snapshot.SessionStatus = string(p.session.status)
		snapshot.SessionInstanceID = p.session.instanceID
		snapshot.SessionExpiresAt = p.session.expiresAt
		snapshot.SessionPosition = p.session.position
		snapshot.SessionQueueDepth = p.session.queueDepth
		snapshot.SessionPollAt = p.session.pollAt
	}
	for agentID, run := range p.runs {
		snapshot.Runs = append(snapshot.Runs, runSnapshot{
			AgentID:      agentID,
			RunID:        run.id,
			StartedAt:    run.startedAt,
			Inflight:     run.inflight,
			RequestCount: run.requestCount,
		})
	}
	return snapshot
}

// AddToken adds a new auth token to the pool at runtime. Returns the pool name.
func (m *RunManager) AddToken(ctx context.Context, token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("token cannot be empty")
	}

	m.mu.Lock()
	for _, p := range m.pools {
		if p.token == token {
			m.mu.Unlock()
			return p.name, errors.New("token already exists")
		}
	}
	name := fmt.Sprintf("token-%d", len(m.pools)+1)
	p := &tokenPool{
		name:   name,
		token:  token,
		cfg:    m.cfg,
		client: m.client,
		runs:   make(map[string]*managedRun),
		logger: m.logger,
	}
	m.pools = append(m.pools, p)
	m.mu.Unlock()

	// Fetch user ID in background (non-blocking for the caller).
	uid, err := m.client.FetchUserID(ctx, token)
	if err != nil {
		m.logger.Printf("%s: fetch user ID failed: %v", name, err)
	} else {
		p.userID = uid
		m.logger.Printf("%s: acting-user-id=%s", name, uid)
	}
	return name, nil
}

// RemoveToken removes a token from the pool by name or full token value.
func (m *RunManager) RemoveToken(ctx context.Context, nameOrToken string) error {
	nameOrToken = strings.TrimSpace(nameOrToken)
	m.mu.Lock()
	var target *tokenPool
	var idx int
	for i, p := range m.pools {
		if p.token == nameOrToken || p.name == nameOrToken {
			target = p
			idx = i
			break
		}
	}
	if target == nil {
		m.mu.Unlock()
		return errors.New("token not found")
	}
	m.pools = append(m.pools[:idx], m.pools[idx+1:]...)
	m.mu.Unlock()

	if err := target.shutdown(ctx); err != nil {
		m.logger.Printf("%s: shutdown on removal failed: %v", target.name, err)
		return err
	}
	m.logger.Printf("%s: removed", target.name)
	return nil
}

type tokenInfo struct {
	Name     string `json:"name"`
	Token    string `json:"token"`
	UserID   string `json:"user_id,omitempty"`
	HasError bool   `json:"has_error"`
}

// ListTokens returns summary info for each configured token.
func (m *RunManager) ListTokens() []tokenInfo {
	m.mu.RLock()
	pools := m.pools
	m.mu.RUnlock()
	out := make([]tokenInfo, 0, len(pools))
	for _, p := range pools {
		p.mu.Lock()
		hasErr := p.lastError != ""
		p.mu.Unlock()
		// Mask token: show first 8 and last 4 chars
		masked := p.token
		if len(masked) > 12 {
			masked = masked[:8] + "..." + masked[len(masked)-4:]
		}
		out = append(out, tokenInfo{
			Name:     p.name,
			Token:    masked,
			UserID:   p.userID,
			HasError: hasErr,
		})
	}
	return out
}
