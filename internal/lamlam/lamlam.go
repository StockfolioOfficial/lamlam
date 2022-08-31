package lamlam

import (
	"context"
	"github.com/stockfolioofficial/lamlam/internal/config"
	"golang.org/x/tools/go/packages"
	"os"
	"path/filepath"
)

type GenerateResult struct {
	PkgPath    string
	OutputPath string
	Content    []byte
	Errs       []error
}

func (gen GenerateResult) Commit() error {
	if len(gen.Content) == 0 {
		return nil
	}
	return os.WriteFile(gen.OutputPath, gen.Content, 0666)
}

func Generate(ctx context.Context, wd string, env []string, cfg *config.Config) ([]GenerateResult, []error) {
	pkgs, errs := load(ctx, wd, env, cfg.LambdaList.Patterns())

	if len(errs) > 0 {
		return nil, errs
	}

	table := makePackageLambdaPair(pkgs, cfg)
	res := make([]GenerateResult, 0, len(table))

	for pkg, lambda := range table {
		gen := makeGen(pkg)

		gr := GenerateResult{
			PkgPath: pkg.PkgPath,
			//TODO: refactor "lamlam_gen.go", 변수화 고민
			OutputPath: filepath.Join(filepath.Dir(gen.intf.FilePath()), "lamlam_gen.go"),
		}
		output, err := generate(gen, lambda)
		if err != nil {
			gr.Errs = append(gr.Errs, err)
		} else {
			gr.Content = output
		}

		res = append(res, gr)
	}

	return res, nil
}

func makePackageLambdaPair(pkgs []*packages.Package, cfg *config.Config) map[*packages.Package]*config.Lambda {
	res := make(map[*packages.Package]*config.Lambda)

	lambdaMap := cfg.LambdaList.ToMap()

	for _, pkg := range pkgs {
		lambda := lambdaMap[pkg.PkgPath]
		if lambda == nil {
			// TODO: error
			continue
		}

		res[pkg] = lambda
	}

	return res
}
