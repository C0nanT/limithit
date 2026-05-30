package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/conantorreswf/limithit/internal/attacks"
)

func RunInteractive(ctx context.Context, stdout, stderr io.Writer) int {
	all := attacks.All()
	opts := make([]huh.Option[string], len(all))
	for i, a := range all {
		opts[i] = huh.NewOption(fmt.Sprintf("%-12s %s", a.Name(), a.Synopsis()), a.Name())
	}

	var attackName string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("limithit — select attack").
				Options(opts...).
				Value(&attackName),
		),
	).Run()

	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return 0
		}
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	a, ok := attacks.Lookup(attackName)
	if !ok {
		fmt.Fprintf(stderr, "error: unknown attack %q\n", attackName)
		return 2
	}

	return runInteractiveForm(ctx, a, stdout, stderr)
}

// runInteractiveForm builds a huh form from a.FormFields(), runs it, then
// dispatches the attack via the TUI live-progress + results model.
func runInteractiveForm(ctx context.Context, a attacks.Attack, stdout, stderr io.Writer) int {
	fields := a.FormFields()

	// Pre-fill defaults into Value pointers.
	for i := range fields {
		if fields[i].Value != nil && *fields[i].Value == "" && fields[i].Default != "" {
			*fields[i].Value = fields[i].Default
		}
	}

	shared := defaultSharedOpts()
	groups := buildFormGroups(fields, &shared)

	form := huh.NewForm(groups...)
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return 0
		}
		fmt.Fprintf(stderr, "error: %s\n", err)
		return 1
	}

	// Check that any FieldWarn acknowledgement was accepted.
	for _, f := range fields {
		if f.Kind == attacks.FieldWarn && f.Value != nil && *f.Value != "true" {
			fmt.Fprintln(stderr, "aborted: safety acknowledgement required")
			return 1
		}
	}

	args := buildArgs(fields, shared)
	return dispatchInteractiveTUI(ctx, a, args, stdout, stderr)
}

// buildFormGroups converts FormFields into huh groups. FieldWarn fields each
// get their own page so they appear as a prominent safety gate.
func buildFormGroups(fields []attacks.FormField, shared *sharedOpts) []*huh.Group {
	var groups []*huh.Group
	var mainFields []huh.Field

	for _, f := range fields {
		if f.Kind == attacks.FieldWarn {
			if len(mainFields) > 0 {
				groups = append(groups, huh.NewGroup(mainFields...))
				mainFields = nil
			}
			groups = append(groups, huh.NewGroup(toHuhFields(f)...))
		} else {
			mainFields = append(mainFields, toHuhFields(f)...)
		}
	}
	if len(mainFields) > 0 {
		groups = append(groups, huh.NewGroup(mainFields...))
	}
	groups = append(groups, sharedOptsGroup(shared))
	return groups
}

// toHuhFields converts one FormField into one or more huh.Field values.
func toHuhFields(f attacks.FormField) []huh.Field {
	switch f.Kind {
	case attacks.FieldWarn:
		v := f.Value
		validate := func(s string) error {
			if s != "true" {
				return errors.New("you must acknowledge the above warning to continue")
			}
			return nil
		}
		return []huh.Field{
			huh.NewNote().Title(f.Label).Description(f.Help),
			huh.NewSelect[string]().
				Title("I acknowledge the above and have authorisation to run this test").
				Options(
					huh.NewOption("Yes — continue", "true"),
					huh.NewOption("No — abort", "false"),
				).
				Value(v).
				Validate(validate),
		}

	case attacks.FieldBool:
		v := f.Value
		return []huh.Field{
			huh.NewSelect[string]().
				Title(f.Label).
				Description(f.Help).
				Options(
					huh.NewOption("Yes", "true"),
					huh.NewOption("No", "false"),
				).
				Value(v),
		}

	case attacks.FieldSelect:
		v := f.Value
		opts := make([]huh.Option[string], 0, len(f.Choices))
		for _, c := range f.Choices {
			if idx := strings.Index(c, "="); idx >= 0 {
				opts = append(opts, huh.NewOption(c[:idx], c[idx+1:]))
			} else {
				opts = append(opts, huh.NewOption(c, c))
			}
		}
		return []huh.Field{
			huh.NewSelect[string]().
				Title(f.Label).
				Description(f.Help).
				Options(opts...).
				Value(v),
		}

	case attacks.FieldURL:
		v := f.Value
		validate := validateURL
		if f.Validate != nil {
			validate = f.Validate
		}
		return []huh.Field{
			huh.NewInput().
				Title(f.Label).
				Description(f.Help).
				Placeholder("https://example.com").
				Value(v).
				Validate(validate),
		}

	default: // FieldString, FieldInt, FieldFloat
		v := f.Value
		field := huh.NewInput().
			Title(f.Label).
			Description(f.Help).
			Value(v)
		if f.Validate != nil {
			field = field.Validate(f.Validate)
		}
		return []huh.Field{field}
	}
}

// buildArgs converts filled FormFields + shared opts into CLI args for runAttack / buildAttackBase.
func buildArgs(fields []attacks.FormField, shared sharedOpts) []string {
	var args []string
	for _, f := range fields {
		if f.Flag == "" || f.Value == nil {
			continue
		}
		v := *f.Value
		switch f.Kind {
		case attacks.FieldWarn:
			if v == "true" {
				args = append(args, "--"+f.Flag)
			}
		case attacks.FieldBool:
			if v == "true" && f.Default != "true" {
				args = append(args, "--"+f.Flag)
			} else if v == "false" && f.Default == "true" {
				args = append(args, "--"+f.Flag+"=false")
			}
		case attacks.FieldString:
			if v != "" {
				args = append(args, "--"+f.Flag, v)
			}
		default: // FieldURL, FieldInt, FieldFloat, FieldSelect
			if v != "" {
				args = append(args, "--"+f.Flag, v)
			}
		}
	}
	args = append(args, shared.args()...)
	return args
}

// sharedOpts holds common engine/output options appended to every interactive form.
type sharedOpts struct {
	outputFmt    string
	outputFile   string
	expectStatus string
	keepalive    bool
}

func defaultSharedOpts() sharedOpts {
	return sharedOpts{
		outputFmt:    "table",
		expectStatus: "0",
		keepalive:    true,
	}
}

func sharedOptsGroup(o *sharedOpts) *huh.Group {
	return huh.NewGroup(
		huh.NewSelect[string]().
			Title("Output format").
			Options(
				huh.NewOption("table (human-readable)", "table"),
				huh.NewOption("json (machine-readable)", "json"),
				huh.NewOption("csv (spreadsheet)", "csv"),
			).
			Value(&o.outputFmt),
		huh.NewInput().
			Title("Output file (empty = stdout)").
			Value(&o.outputFile),
		huh.NewInput().
			Title("Assert HTTP status code (0 = disabled)").
			Description("Exit non-zero if this status code is never observed").
			Value(&o.expectStatus).
			Validate(attacks.ValidateNonNegInt),
		huh.NewConfirm().
			Title("Enable HTTP keep-alive").
			Description("Disable to force a new TCP/TLS handshake per request").
			Value(&o.keepalive),
	)
}

func (o sharedOpts) args() []string {
	var args []string
	if o.outputFmt != "" && o.outputFmt != "table" {
		args = append(args, "--output", o.outputFmt)
	}
	if o.outputFile != "" {
		args = append(args, "--output-file", o.outputFile)
	}
	if o.expectStatus != "0" && o.expectStatus != "" {
		args = append(args, "--expect-status", o.expectStatus)
	}
	if !o.keepalive {
		args = append(args, "--keepalive=false")
	}
	return args
}
