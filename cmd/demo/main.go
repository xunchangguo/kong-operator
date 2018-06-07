package main

import (
	"fmt"
	"net/http"

	kongcli "github.com/xunchangguo/kong-operator/pkg/apis/admin"
	kongadminv1 "github.com/xunchangguo/kong-operator/pkg/apis/admin/v1"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"
)

func main() {
	kongClient, err := kongcli.NewRESTClient(&rest.Config{
		Host:     "http://172.17.80.37:8001",
		Username: "",
		Password: "",
		Timeout:  0,
	})
	if err != nil {
		logrus.Errorf("Error creating Kong Rest client: %v", err)
	}
	upstreamName := "kube-system-echoserver"
	target := "127.0.0.1:8080"
	apiUri := "/echoserver"
	b, res := kongClient.Upstreams().Get(upstreamName)
	if res.StatusCode == http.StatusNotFound {
		upstream := kongadminv1.NewUpstream(upstreamName)
		b, res = kongClient.Upstreams().Create(upstream)
		if res.StatusCode != http.StatusCreated {
			logrus.Errorf("Unexpected error creating Kong Upstream: %v", res)
		}
	}
	fmt.Printf("%v", b)
	kongTargets, err := kongClient.Targets().List(nil, upstreamName)
	if err != nil {
		return
	}
	has := false
	for _, kongTarget := range kongTargets.Items {
		if target == kongTarget.Target {
			has = true
			break
		}
	}
	if has == false {
		target := &kongadminv1.Target{
			Target:   target,
			Upstream: b.ID,
		}
		logrus.Infof("creating Kong Target %v for upstream %v", target, b.ID)
		_, res := kongClient.Targets().Create(target, upstreamName)
		if res.StatusCode != http.StatusCreated {
			logrus.Errorf("Unexpected error creating Kong Target: %v", res)
			return
		}
	}

	//TODO add kongAnnotationApiUriKey
	if apiUri != "" {
		_, res := kongClient.Apis().Get(upstreamName)
		if res.StatusCode == http.StatusNotFound {
			api := &kongadminv1.Api{
				Name:        upstreamName,
				Hosts:       []string{},
				Uris:        []string{apiUri},
				Methods:     []string{"GET", "POST", "DELETE", "PUT", "PATCH", "OPTIONS"},
				UpstreamUrl: fmt.Sprintf("http://%s", upstreamName),
				StripUri:    true,
			}
			logrus.Infof("creating Kong apis %s for upstream %s", apiUri, b.ID)
			_, res := kongClient.Apis().Create(api)
			if res.StatusCode != http.StatusCreated {
				logrus.Errorf("Unexpected error creating Kong Apis: %v", res)
				return
			}
		}
	}
}
