/*
Copyright Ezpie at http://ezpie.ai.
*/

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1 "github.com/ez-pie/ez-supervisor/pkg/generated/clientset/versioned/typed/stable.ezpie.ai/v1"
	rest "k8s.io/client-go/rest"
	testing "k8s.io/client-go/testing"
)

type FakeStableV1 struct {
	*testing.Fake
}

func (c *FakeStableV1) DevWorkspaces(namespace string) v1.DevWorkspaceInterface {
	return &FakeDevWorkspaces{c, namespace}
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *FakeStableV1) RESTClient() rest.Interface {
	var ret *rest.RESTClient
	return ret
}
