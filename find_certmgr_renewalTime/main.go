package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/infacloud/k8s_resources/utils"
	certv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"os/user"
	"sigs.k8s.io/aws-iam-authenticator/pkg/token"
	"sync"
)

var log = utils.GetLogger()
var exceptionNamespacesMap map[string]interface{}

type CertResources struct {
	Name      string
	Namespace string
	RenewTime *metav1.Time
}

var kubeconfig string

func init() {
	flag.StringVar(&kubeconfig, "KUBECONFIG", "/Users/mseelam/k8s/cloudtrust-test1-eks-qa-usw2", "path to Kubernetes config file")
	flag.Parse()
}

func newRESTClientForCertmanager(cluster *eks.Cluster) (*rest.RESTClient, error) {
	var config *rest.Config
	var err error

	if kubeconfig == "" {
		log.Printf("using in-cluster configuration")
		config, err = rest.InClusterConfig()
	} else {
		log.Printf("using configuration from '%s'", kubeconfig)
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	}

	if err != nil {
		panic(err)
	}

	var gv = schema.GroupVersion{Group: "cert-manager.io", Version: "v1"}
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	config.ContentType = runtime.ContentTypeJSON
	config.NegotiatedSerializer = serializer.WithoutConversionCodecFactory{CodecFactory: serializer.NewCodecFactory(&runtime.Scheme{})}
	restClient, err := rest.RESTClientFor(config)
	if err != nil {
		fmt.Println("restClient error:", err)
		return nil, err
	}

	return restClient, nil
}

func newClientset(cluster *eks.Cluster) (*kubernetes.Clientset, *rest.Config, error) {
	gen, err := token.NewGenerator(true, false)
	if err != nil {
		return nil, nil, err
	}
	opts := &token.GetTokenOptions{
		ClusterID: aws.StringValue(cluster.Name),
	}
	tok, err := gen.GetWithOptions(opts)
	if err != nil {
		return nil, nil, err
	}
	ca, err := base64.StdEncoding.DecodeString(aws.StringValue(cluster.CertificateAuthority.Data))
	if err != nil {
		return nil, nil, err
	}
	config := &rest.Config{
		Host:        aws.StringValue(cluster.Endpoint),
		BearerToken: tok.Token,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: ca,
		},
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, err
	}
	return clientset, config, nil
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
	regions := []string{"us-west-2"}

	//regions, _ := GetRegions(sess)
	regionClusterMap := make(map[string]map[string][]*CertResources)
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
		clustersMap := make(map[string][]*CertResources, 0)
		wg := sync.WaitGroup{}
		for _, c := range c {
			wg.Add(1)
			goRoutineForCertificateData(*c, env, r, clusterSvc, clustersMap, &wg)
		}

		wg.Wait()
		regionClusterMap[r] = clustersMap

		writeJson("clusters.json", env, r, clustersMap)

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

func goRoutineForCertificateData(name, env, region string, svc *eks.EKS, clustersMap map[string][]*CertResources, wg *sync.WaitGroup) {
	defer wg.Done()
	resources, err := getCertificateData(name, env, region, svc)
	if err != nil {
		log.Infof("Error getting cluster details [%v]", err)
	}
	lock.Lock()
	defer lock.Unlock()
	clustersMap[name] = resources
}

var nsLock = sync.Mutex{}

func getCertificateData(name, env, region string, svc *eks.EKS) ([]*CertResources, error) {
	log := log.WithField("Cluster", name).WithField("region", region)

	input := &eks.DescribeClusterInput{
		Name: aws.String(name),
	}
	result, err := svc.DescribeCluster(input)
	if err != nil {
		log.Errorf("Error calling DescribeCluster: %v", err)
		return nil, err
	}

	_, config, err := newClientset(result.Cluster)
	if err != nil {
		log.Errorf("Error creating clientSet: %v", err)
		return nil, err
	}

	dynamic1 := dynamic.NewForConfigOrDie(config)
	resourceId := schema.GroupVersionResource{
		Group:    "cert-manager.io",
		Version:  "v1",
		Resource: "certificates",
	}
	list, err := dynamic1.Resource(resourceId).Namespace("").List(context.Background(), metav1.ListOptions{})

	if err != nil {
		return nil, err
	}

	resources := make([]*CertResources, len(list.Items))
	for i, l := range list.Items {
		cert := new(certv1.Certificate)
		//resources.RenewTime = l.sta
		x := getJsonStr(l.Object)
		//fmt.Println(x)
		_ = json.Unmarshal([]byte(x), cert)
		resources[i] = new(CertResources)
		resources[i].Name = cert.Name
		resources[i].Namespace = cert.Namespace
		resources[i].RenewTime = cert.Status.RenewalTime
		fmt.Println(resources[i].Name, resources[i].Namespace, resources[i].RenewTime)
	}

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
