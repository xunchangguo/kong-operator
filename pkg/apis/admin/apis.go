package admin

import (
	"encoding/json"
	"net/url"

	adminv1 "github.com/xunchangguo/kong-operator/pkg/apis/admin/v1"
)

type ApisGetter interface {
	Apis() ApiInterface
}

type ApiInterface interface {
	List(url.Values) (*adminv1.ApiList, error)
	Get(string) (*adminv1.Api, *APIResponse)
	Create(*adminv1.Api) (*adminv1.Api, *APIResponse)
	Delete(string) error
}

type apiAPI struct {
	client APIInterface
}

func (a *apiAPI) Create(api *adminv1.Api) (*adminv1.Api, *APIResponse) {
	out := &adminv1.Api{}
	err := a.client.Create(api, out)
	return out, err
}

func (a *apiAPI) Get(name string) (*adminv1.Api, *APIResponse) {
	out := &adminv1.Api{}
	err := a.client.Get(name, out)
	return out, err
}

func (a *apiAPI) List(params url.Values) (*adminv1.ApiList, error) {
	if params == nil {
		params = url.Values{}
	}

	apiList := &adminv1.ApiList{}
	request := a.client.RestClient().Get().Resource("apis")
	for k, vals := range params {
		for _, v := range vals {
			request.Param(k, v)
		}
	}
	data, err := request.DoRaw()
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, apiList); err != nil {
		return nil, err
	}

	if len(apiList.NextPage) > 0 {
		params.Set("offset", apiList.Offset)
		result, err := a.List(params)
		if err != nil {
			return nil, err
		}
		apiList.Items = append(apiList.Items, result.Items...)
	}

	return apiList, err
}

func (a *apiAPI) Delete(id string) error {
	return a.client.Delete(id)
}
