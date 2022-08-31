package lamlam

import (
	"context"
	"golang.org/x/tools/go/packages"
)

func load(ctx context.Context, wd string, env []string, patterns []string) ([]*packages.Package, []error) {
	cfg := &packages.Config{
		Context: ctx,
		Mode:    packages.LoadAllSyntax,
		Dir:     wd,
		Env:     env,
	}
	escaped := make([]string, len(patterns))
	for i := range patterns {
		escaped[i] = "pattern=" + patterns[i]
	}
	pkgs, err := packages.Load(cfg, escaped...)
	if err != nil {
		return nil, []error{err}
	}
	var errs []error
	for _, p := range pkgs {
		for _, e := range p.Errors {
			errs = append(errs, e)
		}
	}
	if len(errs) > 0 {
		return nil, errs
	}
	return pkgs, nil
}
