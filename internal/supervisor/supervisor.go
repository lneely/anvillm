package supervisor

import (
	"context"
	"time"

	"anvillm/internal/backend/tmux"
	"anvillm/internal/logging"
	"anvillm/internal/p9"
	"anvillm/internal/session"
	"go.uber.org/zap"
)

const idleThreshold = 15 * time.Second

type Supervisor struct {
	sessions *session.Manager
	roles    *p9.RolesFS
}

func New(s *session.Manager, r *p9.RolesFS) *Supervisor {
	return &Supervisor{sessions: s, roles: r}
}

// nudgeIdleWorkers sends a prompt to any worker-role session that has been
// idle for longer than idleThreshold, reminding it to run its polling loop.
func (s *Supervisor) nudgeIdleWorkers() {
	for _, botID := range s.sessions.List() {
		sess := s.sessions.Get(botID)
		if sess == nil || sess.State() != "idle" {
			continue
		}

		tmuxSess, ok := sess.(*tmux.Session)
		if !ok {
			continue
		}

		if tmuxSess.IdleDuration() < idleThreshold {
			continue
		}

		role := tmuxSess.GetRole()
		if role == "" || !s.roles.IsWorker(role) {
			continue
		}

		ctx := context.Background()
		if _, err := sess.Send(ctx, "You are idle. Run your work polling loop to check for available tasks."); err != nil {
			logging.Logger().Warn("supervisor: failed to nudge idle worker",
				zap.String("session", botID), zap.Error(err))
		}
	}
}

func (s *Supervisor) Run(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			logging.Logger().Error("supervisor panic", zap.Any("panic", r))
		}
	}()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.nudgeIdleWorkers()
		}
	}
}
