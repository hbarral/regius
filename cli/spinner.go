package cli

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/mattn/go-isatty"
)

// spinner animates a braille glyph on stderr while a long-running task runs.
// When stderr is not a TTY it falls back to a single static line so logs and
// piped output stay readable (no escape codes, no line redraws).
type spinner struct {
	mu      sync.Mutex
	frames  []rune
	msg     string
	stopCh  chan struct{}
	doneCh  chan struct{}
	enabled bool
}

func newSpinner(msg string) *spinner {
	return &spinner{
		frames:  []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'},
		msg:     msg,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
		enabled: isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd()),
	}
}

func (s *spinner) start() {
	if !s.enabled {
		fmt.Fprintf(os.Stderr, "  • %s ...\n", s.msg)
		return
	}
	go func() {
		defer close(s.doneCh)
		i := 0
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		fmt.Fprintf(os.Stderr, "\r\033[K  %c %s", s.frames[i%len(s.frames)], s.msg)
		for {
			select {
			case <-s.stopCh:
				fmt.Fprintf(os.Stderr, "\r\033[K")
				return
			case <-tick.C:
				s.mu.Lock()
				i++
				fmt.Fprintf(os.Stderr, "\r\033[K  %c %s", s.frames[i%len(s.frames)], s.msg)
				s.mu.Unlock()
			}
		}
	}()
}

func (s *spinner) stop() {
	if !s.enabled {
		return
	}
	close(s.stopCh)
	<-s.doneCh
}
