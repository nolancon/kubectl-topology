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
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"k8s.io/kubernetes/pkg/kubelet/cm/cpuset"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type SystemTopology struct {
	deviceCheckpointFile string
	cpuCheckpointFile    string
	systemCpuTopology    map[int]cpuset.CPUSet
	systemDevices        []DeviceInfo
	pods                 []PodInfo
}

type PodInfo struct {
	podName string
	// Map of pod UID to container info
	podContainers map[string][]ContainerInfo
}

type ContainerInfo struct {
	imageName string
	imageID   string
	cpus      map[int]cpuset.CPUSet
	//devices mapped by image name (imageID not available from device checkpoint)
	devices      map[string][]DeviceInfo
	topologyHint string
}

type DeviceInfo struct {
	name   string
	idInfo map[string][]int64
}

func newSystemTopology() SystemTopology {
	devCheckFile, cpuCheckFile, err := readConfig()
	if err != nil {
		fmt.Println(err)

	}
	devicesInfo := make([]DeviceInfo, 0)
	podCons := make([]PodInfo, 0)
	sysTopo := SystemTopology{
		deviceCheckpointFile: devCheckFile,
		cpuCheckpointFile:    cpuCheckFile,
		systemCpuTopology:    make(map[int]cpuset.CPUSet),
		systemDevices:        devicesInfo,
		pods:                 podCons,
	}
	return sysTopo
}

func readConfig() (string, string, error) {
	defaultCPU := "/var/lib/kubelet/cpu_manager_state"
	defaultDevice := "/var/lib/kubelet/device-plugins/kubelet_internal_checkpoint"
	HOME := os.Getenv("HOME")
	fileName := fmt.Sprintf("%s/.kubectl-topology-config.yaml", HOME)
	configFile, err := ioutil.ReadFile(fileName)
	if err != nil {
		fmt.Println("Using default checkpoint file locations")
		return defaultDevice, defaultCPU, err
	}
	cfg := make(map[string]string)
	err = yaml.Unmarshal(configFile, &cfg)
	if err != nil {
		fmt.Println("Using default checkpoint file locations")
		return defaultDevice, defaultCPU, err
	}

	return cfg["deviceCheckpointFile"], cfg["cpuCheckpointFile"], nil
}

func (st *SystemTopology) printNodeTopology() {
	fmt.Println("Node Device Topology:")
	for _, device := range st.systemDevices {
		fmt.Println("  Name:\t", device.name)
		for id, nodes := range device.idInfo {
			fmt.Println("    ID:\t\t\t", id)
			fmt.Println("    NUMA Nodes:\t\t", nodes)
		}
	}
	fmt.Println("\nNode CPU/NUMA Topology:")
	for numaNode, cpus := range st.systemCpuTopology {
		fmt.Printf("  NUMA Node %d:\n", numaNode)
		fmt.Println("    CPUs:", cpus)
	}
}

func (st *SystemTopology) printAllPodsTopology() {
	fmt.Println("Pods on current node consuming CPU and/or device resources:\n")
	for _, pod := range st.pods {
		printPodTopology(pod)
	}
}

func printPodTopology(pod PodInfo) {
	for podUID, containers := range pod.podContainers {
		fmt.Println("Pod Name:\t\t", pod.podName)
		fmt.Println("  UID:\t\t\t", podUID)
		for _, container := range containers {
			fmt.Println("  Container Name:\t", container.imageName)
			fmt.Println("    ID:\t\t\t", container.imageID)
			fmt.Println("    CPU Resources:")
			for numaNode, cpus := range container.cpus {
				fmt.Printf("      NUMA Node %d:\n", numaNode)
				fmt.Println("      CPUs:\t\t", cpus)
			}
			for _, device := range container.devices[container.imageName] {
				fmt.Println("    Device:\t\t", device.name)
				for id, nodes := range device.idInfo {
					fmt.Println("      ID:\t\t", id)
					fmt.Println("      NUMA Nodes:\t", nodes)
				}
			}
		}
	}
}

// Get pod container IDs and container names from kubectl get pod
func (st *SystemTopology) getAllPodInfo() error {
	getCmd := exec.Command("kubectl", "get", "pod", "-o", "json")
	jsonGetPod, err := getCmd.CombinedOutput()
	if err != nil {
		return err
	}

	var items map[string][]map[string]string
	json.Unmarshal(jsonGetPod, &items)

	var nameUid map[string][]map[string]map[string]string
	json.Unmarshal(jsonGetPod, &nameUid)

	var containerStatuses map[string][]map[string]map[string][]map[string]string
	json.Unmarshal(jsonGetPod, &containerStatuses)

	for i, _ := range items["items"] {
		if items["items"][i]["kind"] == "Pod" {
			skip := false
			pod := PodInfo{}
			pod.podName = nameUid["items"][i]["metadata"]["name"]
			podUid := nameUid["items"][i]["metadata"]["uid"]
			podContainers := make(map[string][]ContainerInfo)
			devices := make(map[string][]DeviceInfo)
			for _, conStatus := range containerStatuses["items"][i]["status"]["containerStatuses"] {
				conInfo := ContainerInfo{}
				imageID := conStatus["containerID"]
				imageName := conStatus["name"]
				conInfo.imageName = imageName
				conInfo.imageID = imageID
				conInfo.cpus, err = st.parseCpuCheckpoint(imageName, podUid)
				if err != nil {
					return err
				}
				devices[imageName], err = st.parseContainerDevices(imageName, podUid)
				if err != nil {
					return err
				}
				if len(conInfo.cpus) == 0 && len(devices[imageName]) == 0 {
					skip = true
				}
				conInfo.devices = devices
				podContainers[podUid] = append(podContainers[podUid], conInfo)
				pod.podContainers = podContainers
			}
			if !skip {
				st.pods = append(st.pods, pod)
			}
		}
	}
	return nil
}

// Parse registered device info from device checkpoint file
func (st *SystemTopology) parseRegisteredDevices() error {
	deviceCheckpoint, err := ioutil.ReadFile(st.deviceCheckpointFile)
	if err != nil {
		return err
	}
	var registeredDevices map[string]map[string]map[string][]string
	json.Unmarshal(deviceCheckpoint, &registeredDevices)

	for registeredDevice, deviceIDs := range registeredDevices["Data"]["RegisteredDevices"] {
		sysDev := DeviceInfo{}
		idInfo := make(map[string][]int64)
		for _, id := range deviceIDs {
			nodes, err := st.getDeviceNUMATopology(id)
			if err != nil {
				return err
			}
			idInfo[id] = nodes
			sysDev.idInfo = idInfo
		}
		sysDev.name = registeredDevice
		st.systemDevices = append(st.systemDevices, sysDev)
	}
	return nil
}

// GetNUMANodeInfo uses sysfs to return a map of NUMANode id to the list of
// CPUs associated with that NUMANode.
func (st *SystemTopology) getNUMATopology() error {
	nodelist, err := ioutil.ReadFile("/sys/devices/system/node/online")
	if err != nil {
		return err
	}

	// Parse the nodelist into a set of Node IDs
	nodes, err := cpuset.Parse(strings.TrimSpace(string(nodelist)))
	if err != nil {
		return err
	}

	info := make(map[int]cpuset.CPUSet)

	// For each node...
	for _, node := range nodes.ToSlice() {
		// Read the 'cpulist' of the NUMA node from sysfs.
		path := fmt.Sprintf("/sys/devices/system/node/node%d/cpulist", node)
		cpulist, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}

		// Convert the 'cpulist' into a set of CPUs.
		cpus, err := cpuset.Parse(strings.TrimSpace(string(cpulist)))
		if err != nil {
			return err
		}

		info[node] = cpus
	}
	st.systemCpuTopology = info
	return nil
}

// getDeviceNUMATopology uses sysfs to get the NUMA node of a device
func (st *SystemTopology) getDeviceNUMATopology(id string) ([]int64, error) {
	numaNodes := make([]int64, 0)
	deviceIDFiles, err := ioutil.ReadDir("/sys/bus/pci/devices/")
	if err != nil {
		return nil, err
	}
	for _, deviceIDFile := range deviceIDFiles {
		deviceIDFileStr := deviceIDFile.Name()
		if strings.HasSuffix(deviceIDFileStr, id) {
			path := fmt.Sprintf("/sys/bus/pci/devices/%s/numa_node", deviceIDFileStr)
			numaNode, err := ioutil.ReadFile(path)
			if err != nil {
				return nil, err
			}
			str := strings.TrimSpace(string(numaNode))
			n, err := strconv.ParseInt(str, 10, 64)
			if err != nil {
				return nil, err
			}
			numaNodes = append(numaNodes, n)
		}
	}
	return numaNodes, nil
}

// Parse container CPUs from CPU checkpoint file
/*func (st *SystemTopology) parseCpuCheckpoint(imageId string) (map[int]cpuset.CPUSet, error) {
	cpuCheckpoint, err := ioutil.ReadFile(st.cpuCheckpointFile)
	if err != nil {
		return nil, err
	}
	var cpuSetStr string
	var result map[string]map[string]string
	json.Unmarshal(cpuCheckpoint, &result)
	for cpuChkimageID, cpus := range result["entries"] {
		if cpuChkimageID == imageId {
			cpuSetStr = cpus
		}
	}
	cpuSet := cpuset.MustParse(cpuSetStr)
	cpuSetSlice := cpuSet.ToSlice()
	cpuSliceMap := make(map[int][]int)
	for _, cpu := range cpuSetSlice {
		for numaNode, sysCpuset := range st.systemCpuTopology {
			if sysCpuset.Contains(cpu) {
				cpuSliceMap[numaNode] = append(cpuSliceMap[numaNode], cpu)
			}
		}
	}
	containerCPUInfo := make(map[int]cpuset.CPUSet)
	for numaNode, cpuSlice := range cpuSliceMap {
		containerCPUInfo[numaNode] = cpuset.NewCPUSet(cpuSlice...)
	}
	return containerCPUInfo, nil
}
*/

func (st *SystemTopology) parseCpuCheckpoint(imageName string, podUID string) (map[int]cpuset.CPUSet, error) {
	cpuCheckpoint, err := ioutil.ReadFile(st.cpuCheckpointFile)
	if err != nil {
		return nil, err
	}
	var cpuSetStr string
	var result map[string]map[string]map[string]string
	json.Unmarshal(cpuCheckpoint, &result)
	for entryPodUID, container := range result["entries"] {
		if entryPodUID == podUID {
			for entryContainerName, cpus := range container {
				if entryContainerName == imageName {
					cpuSetStr = cpus
				}
			}
		}
	}
	cpuSet := cpuset.MustParse(cpuSetStr)
	cpuSetSlice := cpuSet.ToSlice()
	cpuSliceMap := make(map[int][]int)
	for _, cpu := range cpuSetSlice {
		for numaNode, sysCpuset := range st.systemCpuTopology {
			if sysCpuset.Contains(cpu) {
				cpuSliceMap[numaNode] = append(cpuSliceMap[numaNode], cpu)
			}
		}
	}
	containerCPUInfo := make(map[int]cpuset.CPUSet)
	for numaNode, cpuSlice := range cpuSliceMap {
		containerCPUInfo[numaNode] = cpuset.NewCPUSet(cpuSlice...)
	}
	return containerCPUInfo, nil
}

// Parse container devices from device checkpoint file
func (st *SystemTopology) parseContainerDevices(imageName string, podUID string) ([]DeviceInfo, error) {
	deviceCheckpoint, err := ioutil.ReadFile(st.deviceCheckpointFile)
	if err != nil {
		return nil, err
	}
	var deviceId map[string]map[string][]map[string][]string
	var containerName map[string]map[string][]map[string]string
	json.Unmarshal(deviceCheckpoint, &deviceId)
	json.Unmarshal(deviceCheckpoint, &containerName)

	containerDevInfo := make([]DeviceInfo, 0)
	for i, _ := range containerName["Data"]["PodDeviceEntries"] {
		if containerName["Data"]["PodDeviceEntries"][i]["PodUID"] == podUID && containerName["Data"]["PodDeviceEntries"][i]["ContainerName"] == imageName {
			devInfo := DeviceInfo{}
			devInfo.name = containerName["Data"]["PodDeviceEntries"][i]["ResourceName"]
			devInfo.idInfo = st.populateContainerDeviceNUMANodes(deviceId["Data"]["PodDeviceEntries"][i]["DeviceIDs"])
			containerDevInfo = append(containerDevInfo, devInfo)
		}
	}
	return containerDevInfo, nil
}

func (st *SystemTopology) populateContainerDeviceNUMANodes(devIDs []string) map[string][]int64 {
	idInfo := make(map[string][]int64)
	for _, devID := range devIDs {
		for _, sysDev := range st.systemDevices {
			for sysDevId, sysDevNodes := range sysDev.idInfo {
				if sysDevId == devID {
					idInfo[devID] = sysDevNodes
				}
			}
		}
	}
	return idInfo
}
