package broker

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/electric-saw/kafta/internal/pkg/configuration"
	"github.com/electric-saw/kafta/internal/pkg/kafka"
	"github.com/electric-saw/kafta/pkg/cmd/util"
	cmdutil "github.com/electric-saw/kafta/pkg/cmd/util"
	"github.com/spf13/cobra"
)

type clusterConfig struct {
	config   *configuration.Configuration
	brokerId string
}

func NewCmdClusterGetConfig(config *configuration.Configuration) *cobra.Command {
	options := &clusterConfig{config: config}
	cmd := &cobra.Command{
		Use:   "get-configs BROKER_ID (not required)",
		Short: "Show broker configs, by default is used coodinator",
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(options.defaultValue(cmd))
			options.run(cmd)
		},
	}

	return cmd
}

func (o *clusterConfig) defaultValue(cmd *cobra.Command) error {
	args := cmd.Flags().Args()

	if len(args) == 0 {
		conn := kafka.MakeConnection(o.config)
		defer conn.Close()

		brokers := kafka.GetBrokers(conn)

		for _, broker := range brokers {
			if broker.IsController {
				o.brokerId = strconv.FormatInt(int64(broker.ID()), 10)
				break
			}
		}

		if o.brokerId == "" {
			return cmdutil.HelpError(cmd, "Impossible find BrokerId coordinator")
		}
	} else {
		o.brokerId = args[0]
	}

	return nil
}

func (o *clusterConfig) run(cmd *cobra.Command) {
	out := util.GetNewTabWriter(os.Stdout)

	conn := kafka.MakeConnection(o.config)
	defer conn.Close()

	configs := kafka.DescribeBrokerConfig(conn, o.brokerId)
	fmt.Fprintf(out, "NAME\tVALUE\tDEFAULT")

	for _, config := range configs {
		o.printContext(config.Name, cmdutil.Wrap(config.Value, 100), config.Default, out)
	}

	out.Flush()
}

func (o *clusterConfig) printContext(name string, value string, isDefault bool, w io.Writer) {
	fmt.Fprintf(w, "%s\t%s\t%v\n", name, value, isDefault)
}
