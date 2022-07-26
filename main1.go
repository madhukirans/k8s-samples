package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/infacloud/k8s_resources/types"
	"github.com/infacloud/k8s_resources/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"os"
	"os/user"
	"sigs.k8s.io/aws-iam-authenticator/pkg/token"
	"sync"
)

var log = utils.GetLogger()
var exceptionNamespacesMap map[string]interface{}

func newClientset(cluster *eks.Cluster) (*kubernetes.Clientset, error) {
	gen, err := token.NewGenerator(true, false)
	if err != nil {
		return nil, err
	}
	opts := &token.GetTokenOptions{
		ClusterID: aws.StringValue(cluster.Name),
	}
	tok, err := gen.GetWithOptions(opts)
	if err != nil {
		return nil, err
	}
	ca, err := base64.StdEncoding.DecodeString(aws.StringValue(cluster.CertificateAuthority.Data))
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(
		&rest.Config{
			Host:        aws.StringValue(cluster.Endpoint),
			BearerToken: tok.Token,
			TLSClientConfig: rest.TLSClientConfig{
				CAData: ca,
			},
		},
	)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}

var nsMap = make(map[string]int)

func main() {
	AWS_PROFILE := os.Getenv("AWS_PROFILE")
	DEV := os.Getenv("DEV")
	creds := credentials.NewEnvCredentials()
	var err error
	// Retrieve the credentials value
	credValue, err := creds.Get()
	if err != nil {
		// handle error
	}
	fmt.Println(credValue.AccessKeyID, credValue.SessionToken, credValue.SecretAccessKey)

	var sess *session.Session
	if DEV == "dev" {
		sess, err = session.NewSessionWithOptions(session.Options{SharedConfigState: session.SharedConfigEnable, Profile: AWS_PROFILE})
	} else {
		sess, err = session.NewSessionWithOptions(session.Options{SharedConfigState: session.SharedConfigEnable})
	}
	if err != nil {
		log.Error(err)
		return
	}
	env := "qa"
	//regions := []string{"us-west-2"}

	regions, _ := GetRegions(sess)
	regionClusterMap := make(map[string]map[string]*types.ClusterResources)
	regionClustersNameMap := make(map[string][]*string)

	fmt.Println("All clusters:", getJsonStr(regionClustersNameMap))
	//writeJson("../clsuternames.json", env, "", ClustersNameMap)

	for _, r := range regions {
		//	if r != "us-east-1" {
		//		continue
		//	}

		clusterSvc := eks.New(sess, &aws.Config{
			Region: aws.String(r),
		})
		c, err := ListK8sClusters(clusterSvc, r)
		if err != nil {
			log.Info("List clusters error ", err)
			continue
		}
		clustersMap := make(map[string]*types.ClusterResources, 0)
		wg := sync.WaitGroup{}
		for _, c := range c {
			wg.Add(1)
			go goRoutineForClusterData(*c, env, r, clusterSvc, clustersMap, &wg)
		}

		wg.Wait()
		regionClusterMap[r] = clustersMap

		//writeJson("clusters.json", env, r, clustersMap)

	}
	//fmt.Println("ns map:------", getJsonStr(nsMap), "------")
	//fmt.Println(getJsonStr(regionClusterMap))
}

func writeJson(fileName string, env, region string, json1 interface{}) error {
	osUser, err := user.Current()
	if err != nil {
		log.Error(err)
		return err
	}
	dirPath := fmt.Sprintf("%s/Dormantk8SClusters/%s/%s", osUser.HomeDir, env, region)
	_ = os.MkdirAll(dirPath, os.ModePerm)
	jsonFile, err := os.Create(fmt.Sprintf("%s/%s", dirPath, fileName))
	if err != nil {
		log.Error(err)
		return err
	}
	defer jsonFile.Close()

	jsonData, err := json.Marshal(json1)
	if err != nil {
		log.Error(err)
		return err
	}
	_, _ = jsonFile.Write(jsonData)
	jsonFile.Close()
	log.Info("JSON data written to ", jsonFile.Name())
	return nil
}

var lock = sync.Mutex{}

func goRoutineForClusterData(name, env, region string, svc *eks.EKS, clustersMap map[string]*types.ClusterResources, wg *sync.WaitGroup) {
	defer wg.Done()
	resources, err := getClustersData(name, env, region, svc)
	if err != nil {
		log.Infof("Error getting cluster details [%v]", err)
	}
	lock.Lock()
	defer lock.Unlock()
	clustersMap[name] = resources
}

var nsLock = sync.Mutex{}

func getClustersData(name, env, region string, svc *eks.EKS) (*types.ClusterResources, error) {
	log := log.WithField("Cluster", name).WithField("region", region)
	input := &eks.DescribeClusterInput{
		Name: aws.String(name),
	}
	result, err := svc.DescribeCluster(input)
	if err != nil {
		log.Errorf("Error calling DescribeCluster: %v", err)
		return nil, err
	}

	clientSet, err := newClientset(result.Cluster)
	if err != nil {
		log.Errorf("Error creating clientSet: %v", err)
		return nil, err
	}
	resources := new(types.ClusterResources)
	resources.Name = name
	resources.Env = env
	resources.Region = region
	resources.Pods = make(map[string][]string)
	resources.Events = make(map[string][]string)

	//for _, n := range resources.Namespaces {
	pods, err := clientSet.AppsV1().Deployments("ct-operator").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		log.Errorf("Error getting deplyments: %v", err)
		return resources, nil
	}
	fmt.Printf("-------There are %d deployments associated with namespace %s cluster %s\n", len(pods.Items), "ct-operator", name)
	//podsArr := make([]string, 0)
	//for _, p := range pods.Items {
	//	podsArr = append(podsArr, p.Name)
	//}
	//resources.Deployments[n] = podsArr
	//}

	//for _, n := range resources.Namespaces {
	//	events, err := clientSet.CoreV1().Events(n).List(metav1.ListOptions{})
	//	if err != nil {
	//		log.Errorf("Error getting events: %v", err)
	//		return resources, nil
	//	}
	//	log.Printf("There are %d events associated with namespace %s", len(events.Items), n)
	//	podsArr := make([]string, 0)
	//	for _, e := range events.Items {
	//		diffInHours := time.Now().Sub(e.LastTimestamp.Time).Hours()
	//		if diffInHours < constants.EVENT_TIME_THRESHOLD {
	//			podsArr = append(podsArr, fmt.Sprintf("%s\t%s", e.LastTimestamp.Time.String(), e.Name))
	//		}
	//	}
	//	resources.Events[n] = podsArr
	//}
	return resources, nil
}

func GetRegions(sess *session.Session) ([]string, error) {
	svc := ec2.New(sess)
	resultRegions, err := svc.DescribeRegions(nil)
	if err != nil {
		log.Errorf("describe regions error %v", err)
		return nil, err
	}
	regions := make([]string, 0)
	for _, r := range resultRegions.Regions {
		regions = append(regions, *r.RegionName)
	}
	log.Info(regions)
	return regions, err
}

func ListK8sClusters(svc *eks.EKS, region string) ([]*string, error) {
	input := &eks.ListClustersInput{
		MaxResults: aws.Int64(100),
	}
	result, err := svc.ListClusters(input)
	if err != nil {
		log.Errorf("List clusters error [%v]", err)
		return nil, err
	}
	log.Info("List clusters Region:", region, getJsonStr(result.Clusters))
	return result.Clusters, err
}

func getJsonStr(obj interface{}) string {
	b, err := json.Marshal(obj)
	if err != nil {
		return ""
	}
	return string(b)
}
