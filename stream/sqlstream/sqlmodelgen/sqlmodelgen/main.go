package main

import (
	"io"
	"os"
	"text/template"

	"github.com/davecgh/go-spew/spew"

	"github.com/skillian/argparse"
	"github.com/skillian/expr/errors"
	"github.com/skillian/expr/stream/sqlstream/sqlmodelgen"
	"github.com/skillian/logging"
)

var (
	configFile string
	modelFile  string

	logger = logging.GetLogger(
		"sqlmodelgen",
		logging.LoggerHandler(
			new(logging.ConsoleHandler),
			logging.HandlerFormatter(logging.DefaultFormatter{}),
			logging.HandlerLevel(logging.VerboseLevel),
		),
	)
)

type Args struct {
	LogLevel     logging.Level
	ConfigFile   string
	ModelFile    string
	ModelContext sqlmodelgen.ModelContext
	TemplateDir  string
}

func main() {
	var args Args
	parser := argparse.MustNewArgumentParser(
		argparse.Description(
			"Generate models from SQL definitions",
		),
	)
	parser.MustAddArgument(
		argparse.OptionStrings("--log-level"),
		argparse.Action("store"),
		argparse.Choices(
			argparse.Choice{Key: "verbose", Value: logging.VerboseLevel},
			argparse.Choice{Key: "debug", Value: logging.DebugLevel},
			argparse.Choice{Key: "info", Value: logging.InfoLevel},
			argparse.Choice{Key: "warn", Value: logging.WarnLevel},
			argparse.Choice{Key: "error", Value: logging.ErrorLevel},
		),
		argparse.Default(logging.WarnLevel),
		argparse.Help(
			"Specify the logging level (default: %v)",
			"warn",
		),
	).MustBind(&args.LogLevel)
	parser.MustAddArgument(
		argparse.OptionStrings("-t", "--type"),
		argparse.Action("store"),
		argparse.Choices(
			argparse.Choice{
				Key:   "go",
				Value: sqlmodelgen.GoModelContext,
			},
			argparse.Choice{
				Key:   "cs",
				Value: sqlmodelgen.CSModelContext,
			},
		),
	).MustBind(&args.ModelContext)
	parser.MustAddArgument(
		argparse.OptionStrings("-T", "--template-dir"),
		argparse.Action("store"),
		argparse.Default(""),
		argparse.Help(
			"Optional custom template directory",
		),
	).MustBind(&args.TemplateDir)
	parser.MustAddArgument(
		argparse.Dest("configfile"),
		argparse.Action("store"),
		argparse.Help(
			"configuration file from which the model is "+
				"derived",
		),
	).MustBind(&args.ConfigFile)

	parser.MustAddArgument(
		argparse.Dest("modelfile"),
		argparse.Action("store"),
		argparse.Help("output model file"),
	).MustBind(&args.ModelFile)

	parser.MustParseArgs()

	if err := Main(args); err != nil {
		panic(err)
	}
}

func Main(args Args) (Err error) {
	logger.SetLevel(args.LogLevel)
	f, err := os.Open(args.ConfigFile)
	if err != nil {
		return errors.Errorf1From(
			err, "failed to open config file %q",
			args.ConfigFile,
		)
	}
	defer errors.Catch(&Err, f.Close)
	cfg, err := sqlmodelgen.ConfigFromJSON(f, args.ModelContext)
	if err != nil {
		return errors.Errorf1From(
			err, "failed to parse file %v as JSON",
			args.ConfigFile,
		)
	}
	fm := make(template.FuncMap, 8)
	t := sqlmodelgen.AddFuncs(
		template.New("<sqlmodelgen>"), fm, args.ModelContext,
	).Funcs(fm)
	if args.TemplateDir == "" {
		t, err = t.ParseFS(args.ModelContext.FS(), "*.txt")
		if err != nil {
			return errors.Errorf1From(
				err, "failed to parse ModelContext file "+
					"system: %v",
				args.ModelContext.FS(),
			)
		}
	} else {
		t, err = t.ParseFiles(args.TemplateDir, "*.txt")
		if err != nil {
			return errors.Errorf1From(
				err, "failed to parse template directory: %v",
				args.TemplateDir,
			)
		}
	}
	var out io.WriteCloser
	if args.ModelFile == "" {
		out = nopWriteCloser{os.Stdout}
	} else {
		out, err = os.Create(args.ModelFile)
		if err != nil {
			return errors.Errorf1From(
				err, "failed to create output file: %v",
				args.ModelFile,
			)
		}
	}
	if logger.Level() <= logging.VerboseLevel {
		logger.Verbose("configuration:\n\n%v", spew.Sdump(cfg))
	}
	defer errors.Catch(&Err, out.Close)
	if err = t.ExecuteTemplate(out, "0root.txt", cfg); err != nil {
		return errors.Errorf1From(
			err, "error executing template: %v", t,
		)
	}
	return
}

type nopWriteCloser struct{ io.Writer }

func (n nopWriteCloser) Close() error { return nil }
