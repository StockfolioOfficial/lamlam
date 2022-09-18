package main

import (
	"context"
	"flag"
	"github.com/google/subcommands"
	"github.com/stockfolioofficial/lamlam/internal/config"
	"github.com/stockfolioofficial/lamlam/internal/lamlam"
	"gopkg.in/yaml.v3"
	"log"
	"os"
	"strings"
)

const (
	configFileName = "lamlam.yaml"
)

var (
	hasCmds = map[string]bool{
		"commands": true, // builtin
		"help":     true, // builtin
		"flags":    true, // builtin

		"init": true,
		"gen":  true,
		"impl": true,
	}
)

func main() {
	subcommands.Register(subcommands.CommandsCommand(), "")
	subcommands.Register(subcommands.FlagsCommand(), "")
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(&initCmd{}, "")
	subcommands.Register(&genCmd{}, "")
	flag.Parse()

	log.SetFlags(0)
	log.SetPrefix("lamlam: ")
	log.SetOutput(os.Stderr)

	if args := flag.Args(); len(args) == 0 || !hasCmds[args[0]] {
		os.Exit(int(defaultCommand()))
	}
	os.Exit(int(subcommands.Execute(context.Background())))
}

func defaultCommand() subcommands.ExitStatus {
	_, err := os.Stat(configFileName)
	if os.IsNotExist(err) {
		log.Printf("\"%s\" not exists\n", configFileName)
		log.Println("create config file")
		initCmd := &initCmd{}
		return initCmd.Execute(context.Background(), flag.CommandLine)
	}

	log.Printf("\"%s\" exists.\n", configFileName)
	log.Println("Generating lamlam")
	genCmd := &genCmd{}
	return genCmd.Execute(context.Background(), flag.CommandLine)
}

var _ subcommands.Command = (*initCmd)(nil)

type initCmd struct {
}

func (*initCmd) Name() string {
	return "init"
}

func (*initCmd) Synopsis() string {
	return "initialize to \"lamlam\" generate"
}

func (*initCmd) Usage() string {
	return `init [packages]

  Initialize to "lamlam" generate

  Create "lamlam.yaml" file
`
}

func (*initCmd) SetFlags(_ *flag.FlagSet) {
}

func (*initCmd) Execute(ctx context.Context, f *flag.FlagSet, args ...interface{}) subcommands.ExitStatus {
	file, err := os.Create(configFileName)
	if err != nil {
		log.Println("failed to create file ", configFileName)
		log.Println(err)
		return subcommands.ExitFailure
	}
	defer file.Close()
	enc := yaml.NewEncoder(file)
	defer enc.Close()

	err = enc.Encode(config.GetInitConfigVersion1())
	if err != nil {
		log.Println("failed to encode configuration")
		log.Println(err)
		return subcommands.ExitFailure
	}

	return subcommands.ExitSuccess
}

var _ subcommands.Command = (*genCmd)(nil)

type genCmd struct {
}

func (*genCmd) Name() string {
	return "gen"
}

func (*genCmd) Synopsis() string {
	return "generate the lamlam mux and handler"
}

func (*genCmd) Usage() string {
	return `gen [packages]

  gen creates the mux and handler
`
}

func (*genCmd) SetFlags(_ *flag.FlagSet) {

}

func (*genCmd) Execute(ctx context.Context, f *flag.FlagSet, args ...interface{}) subcommands.ExitStatus {
	wd, err := os.Getwd()
	if err != nil {
		log.Println("failed to get working directory: ", err)
		return subcommands.ExitFailure
	}

	_, err = os.Stat(configFileName)
	if os.IsNotExist(err) {
		log.Printf("\"%s\" not exists\n", configFileName)
		log.Println("\"lamlam init\" first")
		return subcommands.ExitFailure
	}

	cfg, err := config.GetFromFile(configFileName)
	if err != nil {
		log.Println("failed to load config file")
		log.Println(err)
	}

	outs, errs := lamlam.Generate(ctx, wd, os.Environ(), cfg)
	if len(errs) > 0 {
		logErrors(errs)
		log.Println("generate failed")
		return subcommands.ExitFailure
	}

	success := true
	for _, out := range outs {

		if len(out.Content) == 0 {
			continue
		}

		if err := out.Commit(); err == nil {
			log.Printf("%s: wrote %s\n", strings.Join(out.PkgPaths, ", "), out.OutputPath)
		} else {
			log.Printf("%s: failed to write %s: %v\n", strings.Join(out.PkgPaths, ", "), out.OutputPath, err)
			success = false
		}
	}

	if !success {
		log.Println("at least one generate failure")
		return subcommands.ExitFailure
	}

	return subcommands.ExitSuccess
}

func logErrors(errs []error) {
	for _, err := range errs {
		log.Println(strings.Replace(err.Error(), "\n", "\n\t", -1))
	}
}
