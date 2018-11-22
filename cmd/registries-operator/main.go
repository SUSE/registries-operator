/*
 * Copyright 2018 SUSE LINUX GmbH, Nuernberg, Germany..
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/golang/glog"
	"github.com/kubic-project/registries-operator/pkg/apis"
	"github.com/kubic-project/registries-operator/pkg/controller"
	"github.com/renstrom/dedent"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	utilflag "k8s.io/apiserver/pkg/util/flag"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	kubeadmutil "k8s.io/kubernetes/cmd/kubeadm/app/util"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"

	regcfg "github.com/kubic-project/registries-operator/pkg/config"
)

// to be set from the build process
var Version string
var Build string

// newCmdManager runs the manager
func newCmdManager(out io.Writer) *cobra.Command {
	var kubeconfigFile = ""

	cmd := &cobra.Command{
		Use:   "manager",
		Short: "Run the Kubic controller manager.",
		Run: func(cmd *cobra.Command, args []string) {
			var err error

			glog.V(1).Infof("[kubic] getting a kubeconfig to talk to the API server")
			if len(kubeconfigFile) > 0 {
				glog.V(3).Infof("[kubic] setting KUBECONFIG to '%s'", kubeconfigFile)
				os.Setenv("KUBECONFIG", kubeconfigFile)
			}
			kubeconfig, err := config.GetConfig()
			kubeadmutil.CheckErr(err)

			glog.V(1).Infof("[kubic] creating a new manager to provide shared dependencies and start components")
			mgr, err := manager.New(kubeconfig, manager.Options{
				//LeaderElection: true,
				//LeaderElectionNamespace: metav1.NamespaceSystem,
			})
			kubeadmutil.CheckErr(err)

			glog.V(1).Infof("[kubic] setting up the scheme for all the resources")
			err = apis.AddToScheme(mgr.GetScheme())
			kubeadmutil.CheckErr(err)

			glog.V(1).Infof("[kubic] setting up all the controllers")
			err = controller.AddToManager(mgr)
			kubeadmutil.CheckErr(err)

			glog.V(1).Infof("[kubic] starting the controller")
			err = mgr.Start(signals.SetupSignalHandler())
			kubeadmutil.CheckErr(err)
		},
	}

	flagSet := cmd.PersistentFlags()
	flagSet.StringVar(&kubeconfigFile, "kubeconfig", "", "Use this kubeconfig file for talking to the API server (not necessary when running in the kuberentes cluster).")
	flagSet.StringVar(&regcfg.DefaultPrefix, "prefix", regcfg.DefaultPrefix, "A prefix for all the resources created by the operator.")
	flagSet.IntVar(&regcfg.DefaultDeployNumReplicas, "replicas", regcfg.DefaultDeployNumReplicas, "Default number of replicas in the Dex Deployment.")

	return cmd
}

func newCmdVersion(out io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version of kubic-init",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(out, "kubic-init version: %s (build: %s)", Version, Build)
		},
	}
	cmd.Flags().StringP("output", "o", "", "Output format; available options are 'yaml', 'json' and 'short'")
	return cmd
}

func main() {
	pflag.CommandLine.SetNormalizeFunc(utilflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	// see https://github.com/kubernetes/kubernetes/issues/17162#issuecomment-225596212
	flag.CommandLine.Parse([]string{})

	pflag.Set("logtostderr", "true")

	cmds := &cobra.Command{
		Use:   "registries-operator",
		Short: "registries-operator: a kubernetes operator for configuring images registries",
		Long: dedent.Dedent(`
			registries-operator: a kubernetes operator for configuring images registries.
		`),
	}

	cmds.ResetFlags()
	cmds.AddCommand(newCmdManager(os.Stdout))
	cmds.AddCommand(newCmdVersion(os.Stdout))

	err := cmds.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
