package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/conantorreswf/limithit/internal/attacks"
	"github.com/conantorreswf/limithit/internal/metrics"
	"github.com/conantorreswf/limithit/internal/report"
)

// doneResult carries the attack result from the run goroutine to the TUI model.
type doneResult struct {
	rep attacks.Report
	err error
}

type progressMsg metrics.Progress
type doneMsg doneResult
type tickMsg time.Time

// runModel is a bubbletea model with two phases:
//
//	0 — live progress display while the attack runs
//	1 — results screen with export/rerun/quit actions
type runModel struct {
	name       string
	args       []string
	stdout     io.Writer
	stderr     io.Writer
	progressCh <-chan metrics.Progress
	doneCh     <-chan doneResult

	phase    int
	spinner  spinner.Model
	progress metrics.Progress
	elapsed  time.Duration
	start    time.Time

	rep        attacks.Report
	err        error
	resultText string
	exported   string
	rerun      bool
	width      int
}

func newRunModel(
	name string,
	args []string,
	progressCh <-chan metrics.Progress,
	doneCh <-chan doneResult,
	stdout, stderr io.Writer,
) runModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return runModel{
		name:       name,
		args:       args,
		stdout:     stdout,
		stderr:     stderr,
		progressCh: progressCh,
		doneCh:     doneCh,
		spinner:    sp,
		start:      time.Now(),
	}
}

func (m runModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		listenProgress(m.progressCh),
		listenDone(m.doneCh),
		tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) }),
	)
}

func (m runModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		}
		if m.phase == 1 {
			switch msg.String() {
			case "q":
				return m, tea.Quit
			case "r":
				m.rerun = true
				return m, tea.Quit
			case "e":
				m.exported = m.exportJSON()
			}
		}

	case spinner.TickMsg:
		if m.phase == 0 {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case tickMsg:
		if m.phase == 0 {
			m.elapsed = time.Since(m.start)
			return m, tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
		}

	case progressMsg:
		m.progress = metrics.Progress(msg)
		return m, listenProgress(m.progressCh)

	case doneMsg:
		m.rep = msg.rep
		m.err = msg.err
		m.phase = 1
		m.resultText = m.buildResultText()
		return m, nil
	}

	return m, nil
}

func (m runModel) View() string {
	if m.phase == 0 {
		return m.viewRunning()
	}
	return m.viewResults()
}

func (m runModel) viewRunning() string {
	var b strings.Builder
	bold := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle().Faint(true)

	fmt.Fprintf(&b, "\n  %s %s\n\n", m.spinner.View(),
		bold.Render("limithit "+m.name))

	p := m.progress
	if p.Total > 0 {
		pct := float64(p.Sent) / float64(p.Total)
		barWidth := 40
		if m.width > 60 {
			barWidth = m.width - 20
		}
		fmt.Fprintf(&b, "  %s %.0f%%\n\n", renderBar(pct, barWidth), pct*100)
		fmt.Fprintf(&b, "  Requests:  %d / %d\n", p.Sent, p.Total)
		fmt.Fprintf(&b, "  RPS:       %.1f\n", p.RPS)
		fmt.Fprintf(&b, "  2xx:       %d\n", p.Success)
		fmt.Fprintf(&b, "  429:       %d\n", p.RateLimited)
		fmt.Fprintf(&b, "  Errors:    %d\n", p.OtherErr)
		fmt.Fprintf(&b, "  Elapsed:   %s\n", p.Elapsed.Round(time.Second))
	} else {
		fmt.Fprintf(&b, "  Elapsed:   %s\n", m.elapsed.Round(time.Second))
	}

	fmt.Fprintf(&b, "\n  %s\n", dim.Render("ctrl+c to interrupt"))
	return b.String()
}

func (m runModel) viewResults() string {
	var b strings.Builder
	if m.err != nil {
		fmt.Fprintf(&b, "\nerror: %s\n\n", m.err)
	} else {
		fmt.Fprintf(&b, "\n%s\n", m.resultText)
	}
	if m.exported != "" {
		fmt.Fprintf(&b, "  exported → %s\n\n", m.exported)
	}
	key := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	fmt.Fprintf(&b, "  %s export JSON   %s run again   %s quit\n",
		key.Render("[e]"), key.Render("[r]"), key.Render("[q]"))
	return b.String()
}

func (m runModel) buildResultText() string {
	if m.rep == nil {
		return "(no report)"
	}
	var buf bytes.Buffer
	if r, ok := m.rep.(*metrics.Report); ok {
		report.Table(&buf, r)
	} else {
		buf.WriteString(m.rep.String())
	}
	return buf.String()
}

// exportJSON writes the report as JSON to an auto-named file and returns the path.
func (m runModel) exportJSON() string {
	r, ok := m.rep.(*metrics.Report)
	if !ok {
		return "(export not supported for this attack type)"
	}
	path := fmt.Sprintf("limithit-%s-%d.json", m.name, time.Now().Unix())
	f, err := os.Create(path)
	if err != nil {
		return fmt.Sprintf("(error: %s)", err)
	}
	defer f.Close()
	if err := report.JSON(f, r); err != nil {
		return fmt.Sprintf("(error: %s)", err)
	}
	return path
}

// renderBar draws a simple ASCII progress bar of the given width (chars).
func renderBar(pct float64, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	filled := int(pct * float64(width))
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}

// listenProgress waits for the next progress value on ch.
// Returns nil if the channel is closed (stops further scheduling).
func listenProgress(ch <-chan metrics.Progress) tea.Cmd {
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return nil
		}
		return progressMsg(p)
	}
}

// listenDone blocks until the attack goroutine sends its result.
func listenDone(ch <-chan doneResult) tea.Cmd {
	return func() tea.Msg {
		return doneMsg(<-ch)
	}
}

// dispatchInteractiveTUI runs the attack with live progress displayed in a bubbletea
// model, then shows an interactive results screen. Returns 0 on success.
// Pressing 'r' re-runs the same attack with the same args.
func dispatchInteractiveTUI(ctx context.Context, a attacks.Attack, args []string, stdout, stderr io.Writer) int {
	progressCh := make(chan metrics.Progress, 8)
	doneCh := make(chan doneResult, 1)

	go func() {
		var parseBuf bytes.Buffer
		base, _, code := buildAttackBase(a, args, &parseBuf)
		if code != 0 {
			close(progressCh)
			logPath := writeInteractiveLog(a.Name(), args, parseBuf.String())
			msg := parseBuf.String()
			if msg == "" {
				msg = fmt.Sprintf("arg parse failed (exit %d)", code)
			}
			if logPath != "" {
				msg = strings.TrimRight(msg, "\n") + "\n\n(details in " + logPath + ")"
			}
			doneCh <- doneResult{err: fmt.Errorf("%s", strings.TrimSpace(msg))}
			return
		}
		base.ProgressCh = progressCh

		rep, err := a.Run(ctx, base)
		// For attacks using worker.Run, the worker has already drained its emitter
		// by the time Run returns, so this close is safe. For raw-socket attacks
		// (slowloris, wsflood) nothing was ever sent to progressCh; closing is also safe.
		close(progressCh)
		doneCh <- doneResult{rep: rep, err: err}
	}()

	m := newRunModel(a.Name(), args, progressCh, doneCh, stdout, stderr)
	prog := tea.NewProgram(m, tea.WithOutput(stderr))
	fm, err := prog.Run()
	if err != nil {
		fmt.Fprintf(stderr, "error: tui: %s\n", err)
		return 1
	}

	final := fm.(runModel)
	if final.rerun {
		fresh, ok := attacks.Lookup(a.Name())
		if !ok {
			return 1
		}
		return dispatchInteractiveTUI(ctx, fresh, args, stdout, stderr)
	}
	return 0
}

// writeInteractiveLog writes a debug log for interactive TUI arg-parse failures.
// Returns the log file path, or "" if writing failed.
func writeInteractiveLog(attackName string, args []string, captured string) string {
	path := fmt.Sprintf("limithit-tui-%s-%d.log", attackName, time.Now().Unix())
	f, err := os.Create(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	fmt.Fprintf(f, "attack: %s\n", attackName)
	fmt.Fprintf(f, "args: %q\n", args)
	fmt.Fprintf(f, "\nstderr output:\n%s\n", captured)
	return path
}
