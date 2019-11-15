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
	"fmt"

	"github.com/spf13/cobra"
)

// podCmd represents the pod command
var podCmd = &cobra.Command{
	Use:   "pod",
	Short: "Display topology of CPU and device resources for pods on the current node.",
	Long: `Display topology of CPU and device resources for pods on the current node.
  Examples:
  # List and display topology of assigned CPUs and devices for 
  all pods on current node consuming CPU and/or devices:
  kubectl topology pod

  # Display topology of assigned CPUs and devices for a specified pod:
  kubectl topology pod <pod-name>

  Note: Only pods on the current node, consuming CPU and/or device
  resources will be considered.`,
	Run: func(cmd *cobra.Command, args []string) {
		st := newSystemTopology()
		st.getNUMATopology()
		st.parseRegisteredDevices()
		st.getAllPodInfo()	
		if len(args) < 1 {
			st.printAllPodsTopology()
			return
		}else if len(args) == 1 {
			st.validPodName(args[0])
			return
		}else {
			fmt.Println("Too many arguments\n\nSee \"kubectl topology --help pod\"")
		}
	},
}

func init() {
	rootCmd.AddCommand(podCmd)
}

func (st *SystemTopology)validPodName(podName string){
	for _, pod := range st.pods {
		if pod.podName == podName {
			printPodTopology(pod)
			return	
		}
	}
	fmt.Println("Invalid request - likely reasons:\n  Incorrect pod name\n\tor\n  Pod is not consuming CPU or Device resources\n")
}

