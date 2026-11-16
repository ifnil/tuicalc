package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"tcalc/tui"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "tcalc [expression...]",
	Short: "terminal graphing calculator",
	Long: `tcalc is a graphing calculator for the terminal.

Each argument is added as an entry, for example:

  tcalc "sin(x)" "a = 2" "f(x) = a x^2 - 1"

Press F1 inside the app for syntax and key bindings.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		m := tui.New(tui.Options{
			Expressions: args,
			Degrees:     viper.GetBool("degrees"),
			Vim:         viper.GetBool("vim"),
		})
		_, err := tea.NewProgram(m).Run()
		return err
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $XDG_CONFIG_HOME/tcalc/config.yaml)")
	rootCmd.Flags().BoolP("degrees", "d", false, "use degrees instead of radians for trig functions")
	rootCmd.Flags().Bool("vim", false, "vim-style modal editing of expressions")
	cobra.CheckErr(viper.BindPFlag("degrees", rootCmd.Flags().Lookup("degrees")))
	cobra.CheckErr(viper.BindPFlag("vim", rootCmd.Flags().Lookup("vim")))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserConfigDir()
		cobra.CheckErr(err)

		viper.AddConfigPath(filepath.Join(home, "/tcalc"))
		viper.AddConfigPath(".")
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")
	}

	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err == nil {
		fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
	}
}
