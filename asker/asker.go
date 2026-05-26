package asker

import (
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"

	"cheater/menu"
)

type HuhAsker struct{}

func New() *HuhAsker { return &HuhAsker{} }

func bail(err error) {
	switch {
	case err == nil:
		return
	case errors.Is(err, huh.ErrUserAborted):
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "\ninput error: %v\n", err)
		os.Exit(1)
	}
}

func (h *HuhAsker) Text(label string) string {
	var out string
	bail(huh.NewInput().Title(label).Value(&out).Run())
	return out
}

func (h *HuhAsker) Required(label string) string {
	var out string
	bail(huh.NewInput().
		Title(label).
		Value(&out).
		Validate(func(s string) error {
			if s == "" {
				return errors.New("required")
			}
			return nil
		}).
		Run())
	return out
}

func (h *HuhAsker) TextWithDefault(label, def string) string {
	out := def
	bail(huh.NewInput().Title(label).Value(&out).Run())
	if out == "" {
		return def
	}
	return out
}

func (h *HuhAsker) Confirm(label string, defaultYes bool) bool {
	out := defaultYes
	bail(huh.NewConfirm().Title(label).Value(&out).Run())
	return out
}

func (h *HuhAsker) Select(label string, options []menu.Option) int {
	return runSelect(label, options)
}

func (h *HuhAsker) SelectCancelable(label string, options []menu.Option) (int, bool) {
	withCancel := append([]menu.Option(nil), options...)
	withCancel = append(withCancel, menu.Option{Title: "(cancel)"})
	idx := runSelect(label, withCancel)
	if idx == len(options) {
		return -1, false
	}
	return idx, true
}

func runSelect(label string, options []menu.Option) int {
	var idx int
	huhOpts := make([]huh.Option[int], len(options))
	for i, o := range options {
		title := o.Title
		if o.Description != "" {
			title = title + "\n      " + o.Description
		}
		huhOpts[i] = huh.NewOption(title, i)
	}
	bail(huh.NewSelect[int]().
		Title(label).
		Options(huhOpts...).
		Value(&idx).
		Height(20).
		Run())
	return idx
}
