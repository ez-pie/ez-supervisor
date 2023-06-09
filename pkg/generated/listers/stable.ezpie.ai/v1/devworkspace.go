/*
Copyright Ezpie at http://ezpie.ai.
*/

// Code generated by lister-gen. DO NOT EDIT.

package v1

import (
	v1 "github.com/ez-pie/ez-supervisor/pkg/apis/stable.ezpie.ai/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// DevWorkspaceLister helps list DevWorkspaces.
// All objects returned here must be treated as read-only.
type DevWorkspaceLister interface {
	// List lists all DevWorkspaces in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1.DevWorkspace, err error)
	// DevWorkspaces returns an object that can list and get DevWorkspaces.
	DevWorkspaces(namespace string) DevWorkspaceNamespaceLister
	DevWorkspaceListerExpansion
}

// devWorkspaceLister implements the DevWorkspaceLister interface.
type devWorkspaceLister struct {
	indexer cache.Indexer
}

// NewDevWorkspaceLister returns a new DevWorkspaceLister.
func NewDevWorkspaceLister(indexer cache.Indexer) DevWorkspaceLister {
	return &devWorkspaceLister{indexer: indexer}
}

// List lists all DevWorkspaces in the indexer.
func (s *devWorkspaceLister) List(selector labels.Selector) (ret []*v1.DevWorkspace, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.DevWorkspace))
	})
	return ret, err
}

// DevWorkspaces returns an object that can list and get DevWorkspaces.
func (s *devWorkspaceLister) DevWorkspaces(namespace string) DevWorkspaceNamespaceLister {
	return devWorkspaceNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// DevWorkspaceNamespaceLister helps list and get DevWorkspaces.
// All objects returned here must be treated as read-only.
type DevWorkspaceNamespaceLister interface {
	// List lists all DevWorkspaces in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1.DevWorkspace, err error)
	// Get retrieves the DevWorkspace from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1.DevWorkspace, error)
	DevWorkspaceNamespaceListerExpansion
}

// devWorkspaceNamespaceLister implements the DevWorkspaceNamespaceLister
// interface.
type devWorkspaceNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all DevWorkspaces in the indexer for a given namespace.
func (s devWorkspaceNamespaceLister) List(selector labels.Selector) (ret []*v1.DevWorkspace, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.DevWorkspace))
	})
	return ret, err
}

// Get retrieves the DevWorkspace from the indexer for a given namespace and name.
func (s devWorkspaceNamespaceLister) Get(name string) (*v1.DevWorkspace, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1.Resource("devworkspace"), name)
	}
	return obj.(*v1.DevWorkspace), nil
}
