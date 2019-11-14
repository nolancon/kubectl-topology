/*
Copyright © 2019 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// nodeCmd represents the node command
var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Display topology of CPU and device resources for the current node.",
	Long: `Display topology of CPU and device resources for the current node:

  kubectl topo node
`,
	Run: func(cmd *cobra.Command, args []string) {
		node()
	},
}

func init() {
	topoCmd.AddCommand(nodeCmd)
}

func node() {
        st := newSystemTopology()
        st.getNUMATopology()
        st.parseRegisteredDevices()
	st.printNodeTopology()
}


