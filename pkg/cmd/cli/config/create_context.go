package config

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	cliflag "github.com/electric-saw/kafta/internal/pkg/flag"
	"github.com/electric-saw/kafta/internal/pkg/kafka"

	"github.com/electric-saw/kafta/internal/pkg/ui"

	"github.com/Shopify/sarama"
	"github.com/electric-saw/kafta/internal/pkg/configuration"
	cmdutil "github.com/electric-saw/kafta/pkg/cmd/util"
	"github.com/spf13/cobra"
)

type createContextOptions struct {
	config           *configuration.Configuration
	name             string
	currContext      bool
	schemaRegistry   cliflag.StringFlag
	ksql             cliflag.StringFlag
	bootstrapServers cliflag.StringFlag
	version          cliflag.StringFlag
	user             cliflag.StringFlag
	password         cliflag.StringFlag
	useSASL          cliflag.StringFlag
	algorithm        cliflag.StringFlag
	useTLS           bool
	parsedVersion    string
	quiet            bool
}

var (
	createContextLong = `
		Sets a context entry in config

		Specifying a name that already exists will merge new fields on top of existing values for those fields.`

	createContextExample = `
		# Set the cluster field on the kafka-dev context entry without touching other values
		kafta config set-context kafka-dev --server=b-1.kafka.example.com,b-2.kafka.example.com,b-3.kafka.example.com`
)

func NewCmdConfigSetContext(config *configuration.Configuration) *cobra.Command {
	options := &createContextOptions{config: config}

	cmd := &cobra.Command{
		Use:                   "set-context [NAME | --current] [--server=server] [--cluster=cluster_nickname] [--schema-registry=url] [--ksql=url]",
		DisableFlagsInUseLine: true,
		Short:                 "Sets a context entry in config",
		Long:                  createContextLong,
		Example:               createContextExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(options.complete(cmd))
			name, exists, err := options.run()
			cmdutil.CheckErr(err)
			if exists {
				fmt.Printf("Context %q modified.\n", name)
			} else {
				fmt.Printf("Context %q created.\n", name)
			}
		},
	}

	cmd.Flags().BoolVar(&options.currContext, "current", options.currContext, "Modify the current context")
	cmd.Flags().Var(&options.schemaRegistry, "schema-registry", "schema-registry for the context")
	cmd.Flags().Var(&options.ksql, "ksql", "ksql for the context")
	cmd.Flags().Var(&options.bootstrapServers, "server", "server for the cluster entry in Kaftaconfig")
	cmd.Flags().Var(&options.version, "version", "kafka vesion for the cluster entry in Kaftaconfig")
	cmd.Flags().Var(&options.useSASL, "sasl", "Use SASL")
	cmd.Flags().VarP(&options.algorithm, "algorithm", "a", "algorithm for SASL")
	cmd.Flags().VarP(&options.user, "username", "u", "Username")
	cmd.Flags().VarP(&options.password, "password", "p", "Password")
	cmd.Flags().BoolVar(&options.useTLS, "tls", true, "Use TLS")

	return cmd
}

func (o *createContextOptions) run() (string, bool, error) {
	err := o.validate()
	if err != nil {
		return "", false, err
	}

	name := o.name
	if o.currContext {
		if len(o.config.KaftaData.CurrentContext) == 0 {
			return "", false, errors.New("no current context is set")
		}
		name = o.config.KaftaData.CurrentContext
	}

	startingInstance, exists := o.config.KaftaData.Contexts[name]
	if !exists {
		startingInstance = configuration.MakeContext()
	}
	cmdutil.CheckErr(o.promptNeeded(startingInstance))

	context := o.modifyContext(*startingInstance)

	fmt.Printf("\n\n%v\n\n", context)

	err = o.checkConnection(context)
	if err != nil {
		cmdutil.CheckErr(err)
		return "", false, fmt.Errorf("could not connect to %s", context.BootstrapServers)
	}

	o.config.KaftaData.Contexts[name] = context

	return name, exists, nil
}

func (o *createContextOptions) modifyContext(context configuration.Context) *configuration.Context {
	modifiedContext := context

	if o.ksql.Provided() {
		modifiedContext.Ksql = o.ksql.Value()
	}

	if o.schemaRegistry.Provided() {
		modifiedContext.SchemaRegistry = o.schemaRegistry.Value()
	}

	if o.bootstrapServers.Provided() {
		modifiedContext.BootstrapServers = strings.Split(o.bootstrapServers.Value(), ",")
	}

	if len(o.parsedVersion) > 0 {
		version, err := sarama.ParseKafkaVersion(o.parsedVersion)
		cmdutil.CheckErr(err)
		modifiedContext.KafkaVersion = version.String()
	}

	if o.useSASL.Provided() {
		modifiedContext.UseSASL = true
	}

	if o.algorithm.Provided() {
		modifiedContext.SASL.Algorithm = o.algorithm.Value()
	}

	if o.user.Provided() {
		modifiedContext.SASL.Username = o.user.Value()
	}

	if o.password.Provided() {
		modifiedContext.SASL.Password = o.password.Value()
	}

	modifiedContext.TLS = o.useTLS

	return &modifiedContext
}

func (o *createContextOptions) complete(cmd *cobra.Command) error {
	args := cmd.Flags().Args()
	if len(args) > 1 {
		return cmdutil.HelpErrorf(cmd, "Unexpected args: %v", args)
	}
	if len(args) == 1 {
		o.name = args[0]
	}

	if o.version.Provided() {
		o.parsedVersion = o.version.Value()
	}

	return nil
}

func (o *createContextOptions) validate() error {
	if len(o.name) == 0 && !o.currContext {
		return errors.New("you must specify a non-empty context name or --current")
	}
	if len(o.name) > 0 && o.currContext {
		return errors.New("you cannot specify both a context name and --current")
	}

	if o.ksql.Provided() && !testHost(o.ksql.String()) {
		return errors.New("failed to connect on ksql")
	}

	if o.schemaRegistry.Provided() && !testHost(o.schemaRegistry.String()) {
		return errors.New("failed to connect on schema-registry")
	}

	if o.useSASL.Provided() && o.quiet {
		if !o.user.Provided() {
			return errors.New("user flag is required if SASL is provided")
		}

		if !o.password.Provided() {
			return errors.New("user flag is required if SASL is provided")
		}
	}

	return nil
}

func testHost(address string) bool {
	if len(strings.Split(address, ":")) <= 1 {
		fmt.Printf("Port is nedeed on %s!\n", address)
		return false
	}
	conn, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		return false
	} else {
		if conn != nil {
			_ = conn.Close()
			return true
		} else {
			return true
		}
	}
}

func (o *createContextOptions) checkConnection(context *configuration.Context) error {
	if o.bootstrapServers.Provided() {
		conn, err := kafka.MakeConnectionContext(o.config, context)
		if err != nil {
			return err
		}
		conn.Close()
	}

	return nil
}

func (o *createContextOptions) promptNeeded(context *configuration.Context) error {
	if o.quiet {
		return nil
	}

	if !o.bootstrapServers.Provided() && len(context.BootstrapServers) == 0 {
		servers, err := ui.GetText("Bootstrap servers", true)
		cmdutil.CheckErr(err)
		err = o.bootstrapServers.Set(servers)
		cmdutil.CheckErr(err)
	}

	if !o.version.Provided() && len(context.KafkaVersion) == 0 {
		version, err := ui.GetText("Kafka version", true)
		cmdutil.CheckErr(err)
		err = o.version.Set(version)
		cmdutil.CheckErr(err)
	}

	if !o.useSASL.Provided() {
		sasl, err := ui.GetConfirmation("Use SASL", false)
		cmdutil.CheckErr(err)
		if sasl {
			err := o.useSASL.Set("true")
			cmdutil.CheckErr(err)

			if !o.algorithm.Provided() && len(context.SASL.Algorithm) == 0 {
				algorithm, err := ui.GetText("SASL Algorithm", true)
				cmdutil.CheckErr(err)
				err = o.algorithm.Set(algorithm)
				cmdutil.CheckErr(err)
			}

			if !o.user.Provided() && len(context.SASL.Username) == 0 {
				user, err := ui.GetText("User", true)
				cmdutil.CheckErr(err)
				err = o.user.Set(user)
				cmdutil.CheckErr(err)
			}

			if !o.password.Provided() && len(context.SASL.Password) == 0 {
				password, err := ui.GetPassword("Password", true)
				cmdutil.CheckErr(err)
				err = o.password.Set(password)
				cmdutil.CheckErr(err)
			}

		}
	}

	// TODO: implement communication with service API
	// if !o.schemaRegistry.Provided() {
	// 	schemaRegistry, err := ui.GetText("Schema registry", false)
	// 	cmdutil.CheckErr(err)
	// 	err = o.schemaRegistry.Set(schemaRegistry)
	// 	cmdutil.CheckErr(err)
	// }
	// if !o.ksql.Provided() {
	// 	ksql, err := ui.GetText("KSQL", false)
	// 	cmdutil.CheckErr(err)
	// 	err = o.ksql.Set(ksql)
	// 	cmdutil.CheckErr(err)
	// }

	return nil
}
