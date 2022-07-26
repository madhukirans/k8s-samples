package types

type NodeStatus struct {
	State   string
	Message string
}

type Node struct {
	Name   string
	Status NodeStatus
	Size string
}
type ClusterResources struct {
	Name       string
	Env        string
	Region     string
	Nodes      []Node
	Namespaces []string
	Pods       map[string][]string
	Deployments map[string][]string
	Events     map[string][]string
	PVs        []string
}

type Resources struct {
	Pods   []string
	Events []string
}

type DormantCluster struct {
	RequestId     string `json:"requestId" dynamodbav:"requestId"`
	Name          string
	Region        string
	Env           string
	TotalPods     int
	TotalEvents   int
	TotalNodes    int
	Nodes         []Node
	Namespaces    map[string]Resources
	MarkForDelete bool
}
