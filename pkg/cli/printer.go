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
	"encoding/json"
	"fmt"

	api "github.com/elemir/etcdops/api/v1alpha1"
	"github.com/jedib0t/go-pretty/v6/text"

	"github.com/jedib0t/go-pretty/v6/table"
	"gopkg.in/yaml.v3"
)

type Prettifier interface {
	Prettify() interface{}
}

type Tabler interface {
	Header() table.Row
	Rows() []table.Row
}

var (
	_ Tabler = api.ClusterList{}
)

func PrettyPrint(obj interface{}, output string) error {
	if t, ok := obj.(Tabler); ok && output == "text" {
		return printTable(t)
	}

	if pObj, ok := obj.(Prettifier); ok {
		obj = pObj.Prettify()
	}

	var buf []byte
	var err error

	if output == "json" {
		if buf, err = json.Marshal(obj); err != nil {
			return err
		}
	} else {
		if buf, err = yaml.Marshal(obj); err != nil {
			return err
		}
	}

	fmt.Printf("%s\n", buf)

	return nil
}

func printTable(obj Tabler) error {
	var t table.Table
	var columnConfigs []table.ColumnConfig

	header := obj.Header()
	for i := range header {
		columnConfigs = append(columnConfigs, table.ColumnConfig{
			Number:      i + 1,
			AlignHeader: text.AlignCenter,
		})
	}

	t.SetColumnConfigs(columnConfigs)
	t.AppendHeader(header)
	t.AppendRows(obj.Rows())

	fmt.Println(t.Render())

	return nil
}
