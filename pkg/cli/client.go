/*
Copyright 2022 Evgenii Omelchenko.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, version 3.

This program is distributed in the hope that it will be useful, but
WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU
General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program. If not, see <http://www.gnu.org/licenses/>.
*/

package cli

import (
	"context"
	"fmt"
	"time"

	etcdops "github.com/elemir/etcdops/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/watch"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	scheme  = runtime.NewScheme()
	spinner = []string{".  ", ".. ", "..."}
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(api.AddToScheme(scheme))
	utilruntime.Must(etcdops.AddToScheme(scheme))
}

type Client struct {
	client.WithWatch
	Namespace string
}

func NewClient() (*Client, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	namespace, _, err := kubeConfig.Namespace()
	if err != nil {
		return nil, err
	}
	cl, err := client.NewWithWatch(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, err
	}

	return &Client{
		WithWatch: cl,
		Namespace: namespace,
	}, nil
}

type Watcher struct {
	watcher watch.Interface
	ticker  *time.Ticker
}

func (cl *Client) Watch(ctx context.Context, list client.ObjectList) (*Watcher, error) {
	ticker := time.NewTicker(time.Second)
	watcher, err := cl.WithWatch.Watch(ctx, list)
	if err != nil {
		ticker.Stop()
		return nil, err
	}

	return &Watcher{
		watcher: watcher,
		ticker:  ticker,
	}, nil
}

type WaitCondition func(watch.Event) bool

func (w *Watcher) Wait(condition WaitCondition) runtime.Object {
	done := make(chan runtime.Object)

	go func() {
		for event := range w.watcher.ResultChan() {
			if condition(event) {
				done <- event.Object
				return
			}
		}
	}()

	i := 0
	now := time.Now()
	for {
		select {
		case obj := <-done:
			fmt.Printf("\rdone (%s)\n", time.Now().Sub(now).Round(time.Second))
			return obj
		case tick := <-w.ticker.C:
			fmt.Printf("\r%s %s", spinner[i%len(spinner)], tick.Sub(now).Round(time.Second))
			i++
		}
	}
}

func (w *Watcher) Stop() {
	w.watcher.Stop()
	w.ticker.Stop()
}
