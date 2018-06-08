package main

import (
	"fmt"
	"net/http"
	"net/url"

	kongcli "github.com/xunchangguo/kong-operator/pkg/apis/admin"
	kongadminv1 "github.com/xunchangguo/kong-operator/pkg/apis/admin/v1"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"
)

func main() {
	kongClient, err := kongcli.NewRESTClient(&rest.Config{
		Host:     "http://192.168.137.61:8001",
		Username: "",
		Password: "",
		Timeout:  0,
	})
	if err != nil {
		logrus.Errorf("Error creating Kong Rest client: %v", err)
	}
	upstreamName := "kube-system-echoserver1"
	//	target := "127.0.0.1:8080"
	apiUri := "/echoserver1"
	apits, err := kongClient.Apis().List(url.Values{
		"upstream_url": []string{"http://kube-system-echoserver"},
	})
	if err != nil {
		logrus.Errorf("Error List apis: %v", err)
		fmt.Errorf("%v", err)
	} else {
		for _, api := range apits.Items {
			fmt.Printf("%v", api)
			logrus.Infof("%v", err)
		}
	}

	api := &kongadminv1.Api{
		Name:        upstreamName,
		Hosts:       map[string]string{},
		Uris:        []string{apiUri},
		Methods:     []string{"GET", "POST", "DELETE", "PUT", "PATCH", "OPTIONS"},
		UpstreamUrl: fmt.Sprintf("http://%s", upstreamName),
		StripUri:    true,
	}
	_, res := kongClient.Apis().Create(api)
	if res.StatusCode != http.StatusCreated {
		logrus.Errorf("Unexpected error creating Kong Apis: %v", res)
	}

}
