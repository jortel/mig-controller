package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/konveyor/mig-controller/pkg/controller/discovery/model"
	"github.com/konveyor/mig-controller/pkg/controller/discovery/web"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/types"
	"net/http"
	liburl "net/url"
	"strings"
)

//
// Examples:
/*
client := Client{
	Token: "token",
}

list := &PvList{}
err := client.List(
	list,
	ListOptions{
		Cluster: Key{
			Namespace: "openshift-migration",
			Name: "host",
		},
	})

pv := &PV{}
err := client.Get(
	pv,
	GetOptions{
		Cluster: Key{
			Namespace: "openshift-migration",
			Name: "host",
		},
		Key: Key{
			Name: "nfs18",
		},
	})
*/

//
// Resources.
type Cluster = web.Cluster
type ClusterList = web.ClusterList
type Review = web.Review
type Plan = web.Plan
type PlanList = web.PlanList
type Namespace = web.Namespace
type NamespaceList = web.NamespaceList
type Pod = web.Pod
type PodList = web.Pod
type PV = web.PV
type PvList = web.PvList
type PVC = web.PVC
type PvcList = web.PvcList
type Service = web.Service
type ServiceList = web.ServiceList

//
// Pagination
type Page model.Page

//
// Options for List().
type Key = types.NamespacedName
type ListOptions struct {
	// Cluster
	Cluster Key
	// Namespace
	Namespace string
	// Pagination
	Page *Page
}

//
// Get the URL path for the resource.
func (s *ListOptions) path(r Resource) string {
	url := r.Path()
	url = strings.Replace(url, ":ns1", s.Cluster.Namespace, 1)
	url = strings.Replace(url, ":cluster", s.Cluster.Name, 1)
	url = strings.Replace(url, ":ns2", s.Namespace, 1)
	return url
}

//
// Options for Get().
type GetOptions struct {
	// Cluster
	Cluster Key
	// Name
	Key Key
}

//
// Get the URL path for the resource.
func (s *GetOptions) path(r Resource) string {
	path := r.Path()
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return path
	}
	path = strings.Replace(path, parts[len(parts)-1], s.Key.Name, 1)
	path = strings.Replace(path, ":ns1", s.Cluster.Namespace, 1)
	path = strings.Replace(path, ":cluster", s.Cluster.Name, 1)
	path = strings.Replace(path, ":ns2", s.Key.Namespace, 1)

	return path
}

//
// Options for Post().
type PostOptions struct {
	// Cluster
	Cluster Key
}

//
// Get the URL path for the resource.
func (s *PostOptions) path(r Resource) string {
	path := r.Path()
	path = strings.Replace(path, ":ns1", s.Cluster.Namespace, 1)
	path = strings.Replace(path, ":cluster", s.Cluster.Name, 1)
	return path
}

//
// REST Resource.
type Resource interface {
	// The URL path.
	Path() string
}

//
// Thin REST API client.
type Client struct {
	// Bearer token.
	Token string
	// Host <host>:<port>
	Host string
}

//
// List resources.
func (c *Client) List(r Resource, options ListOptions) error {
	path := options.path(r)
	return c.get(path, r)
}

//
// Get a resource.
func (c *Client) Get(r Resource, options GetOptions) error {
	path := options.path(r)
	return c.get(path, r)
}

//
// Post a resource.
func (c *Client) Post(r Resource, options PostOptions) error {
	path := options.path(r)
	return c.post(path, r)
}

//
// Get the URL.
func (c *Client) url(path string) *liburl.URL {
	if c.Host == "" {
		c.Host = "localhost:8080"
	}
	url, _ := liburl.Parse(path)
	if url.Host == "" {
		url.Scheme = "http"
		url.Host = c.Host
	}

	return url
}

//
// Http GET
func (c *Client) get(path string, resource Resource) error {
	header := http.Header{}
	if c.Token != "" {
		header["Authorization"] = []string{
			fmt.Sprintf("Bearer %s", c.Token),
		}
	}
	request := &http.Request{
		Method: http.MethodGet,
		Header: header,
		URL:    c.url(path),
	}
	client := http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	if response.StatusCode == http.StatusOK {
		defer response.Body.Close()
		content, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return err
		}
		err = json.Unmarshal(content, resource)
		if err != nil {
			return err
		}
		return nil
	}

	return errors.New(response.Status)
}

//
// Http POST
func (c *Client) post(path string, resource Resource) error {
	header := http.Header{}
	if c.Token != "" {
		header["Authorization"] = []string{
			fmt.Sprintf("Bearer %s", c.Token),
		}
	}
	body, _ := json.Marshal(resource)
	reader := bytes.NewReader(body)
	request := &http.Request{
		Body:   ioutil.NopCloser(reader),
		Method: http.MethodPost,
		Header: header,
		URL:    c.url(path),
	}
	client := http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	if response.StatusCode == http.StatusCreated {
		defer response.Body.Close()
		content, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return err
		}
		err = json.Unmarshal(content, resource)
		if err != nil {
			return err
		}
		return nil
	}

	return errors.New(response.Status)
}
