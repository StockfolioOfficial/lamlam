package lamlam

import (
	"context"
	"errors"
	"github.com/stockfolioofficial/lamlam/internal/config"
	"os"
	"path/filepath"
)

type GenerateResult struct {
	PkgPaths   []string
	OutputPath string
	Content    []byte
}

func (gen GenerateResult) Commit() error {
	if len(gen.Content) == 0 {
		return nil
	}

	err := os.MkdirAll("./"+filepath.Dir(gen.OutputPath), os.ModePerm)
	if err != nil {
		return err
	}
	return os.WriteFile(gen.OutputPath, gen.Content, 0666)
}

func Generate(ctx context.Context, wd string, env []string, cfg *config.Config) ([]GenerateResult, []error) {
	res := make([]GenerateResult, 0, len(cfg.LambdaList))
	for i := range cfg.LambdaList {
		lambda := &cfg.LambdaList[i]

		if len(lambda.Type) == 0 {
			return nil, []error{errors.New("empty interface type")}
		}

		var gr GenerateResult

		//TODO: refactor "lamlam_gen.go", 변수화 고민
		gr.OutputPath = filepath.Join(lambda.Output, "lamlam_gen.go")
		gr.PkgPaths = make([]string, 0, len(lambda.Type))
		for _, typ := range lambda.Type {
			pkgPath, err := typ.GetPackagePath()
			if err != nil {
				return nil, []error{err}
			}

			gr.PkgPaths = append(gr.PkgPaths, pkgPath)
		}
		pkgs, errs := load(ctx, wd, env, gr.PkgPaths)
		if len(errs) > 0 {
			return nil, errs
		}

		var err error
		gr.Content, err = gen(pkgs, lambda)
		if err != nil {
			return nil, []error{err}
		}

		res = append(res, gr)
	}

	return res, nil
}
