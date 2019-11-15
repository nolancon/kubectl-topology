# node-topology
This is a `kubectl` plugin to view the topology of CPU and device resources on the current node. `kubectl topo` uses the local CPU Manager and Device Manager checkpoint files to access resource information. Therefore this is a node level application and the plugin is limited to the node on which you are logged in.
## Usage
- Display topology of CPU and device resources for the current node:
`kubectl topo node`
- Display topology of assigned CPUs and devices for all pods on current node consuming CPU and/or devices:
`kubectl topo pod`
- Display topology of assigned CPUs and devices for a specified pod:
`kubectl topo pod <pod-name>`
  #### Note: Only pods on the current node, consuming CPU and/or device resources will be considered by the plugin.
  
## Download
  
## Configuration
