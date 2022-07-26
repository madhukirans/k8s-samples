package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/config"
	//"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds
)

func main() {
	//creds := credentials.NewEnvCredentials()
	//var err error
	// Retrieve the credentials value
	//credValue, err := creds.Get()
	//if err != nil {
	//	fmt.Println(err)
	//}

	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		fmt.Println("failed to load configuration, %v", err)
	}
	cr, err := cfg.Credentials.Retrieve(context.TODO())
	fmt.Println("------", err, getJsonStr(cr))

	//creds := credentials.NewCredentials(&ec2rolecreds.EC2RoleProvider{
	//	Expiry:       credentials.Expiry{},
	//	Client:       nil,
	//	ExpiryWindow: 0,
	//})
	////creds.Expire()
	//credsValue, err := creds.Get()
	//
	//fmt.Println(err)
	//fmt.Println(credsValue.AccessKeyID, credsValue.SessionToken, credsValue.SecretAccessKey)
}

//func getJsonStr(obj interface{}) string {
//	b, err := json.Marshal(obj)
//	if err != nil {
//		return ""
//	}
//	return string(b)
//}
