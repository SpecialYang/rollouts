/*
Copyright 2022 The Kruise Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package luamanager

import (
	"encoding/json"
	"fmt"
	"testing"

	rolloutv1alpha1 "github.com/openkruise/rollouts/api/v1alpha1"
	"github.com/openkruise/rollouts/pkg/util"
	lua "github.com/yuin/gopher-lua"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	luajson "layeh.com/gopher-json"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestRunLuaScript(t *testing.T) {
	cases := []struct {
		name         string
		getObj       func() *unstructured.Unstructured
		getLuaScript func() string
		expectResult func() string
	}{
		{
			name: "lua script test 1",
			getObj: func() *unstructured.Unstructured {
				type LuaData struct {
					Annotations map[string]string
					Weight      string
					Matches     []rolloutv1alpha1.HttpRouteMatch
				}
				data := &LuaData{
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "nginx",
					},
					Weight: "0",
					Matches: []rolloutv1alpha1.HttpRouteMatch{
						{
							Headers: []gatewayv1alpha2.HTTPHeaderMatch{
								{
									Name:  "user_id",
									Value: "123456",
								},
							},
						},
					},
				}
				unObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(data)
				if err != nil {
					fmt.Println("to unstructured failed", err.Error())
				}
				obj := &unstructured.Unstructured{Object: unObj}
				return obj
			},
			getLuaScript: func() string {
				return `
					annotations = obj.annotations
					annotations["weight"] = obj.weight
					if ( not obj.matches )
					then
						return annotations
					end
					for _,match in pairs(obj.matches)
					do
						if ( not (match or match.headers) )
						then
							return annotations
						end
						for _,header in pairs(match.headers)
						do
							if ( not header )
							then
								return annotations
							end
							condition = {}
							condition["name"] = header.name
							condition["value"] = header.value
							annotations["condition"] = json.encode(condition)
						end
					end
					return annotations
				`
			},
			expectResult: func() string {
				obj := map[string]string{
					"weight":                      "0",
					"kubernetes.io/ingress.class": "nginx",
					"condition":                   `{"name":"user_id","value":"123456"}`,
				}
				return util.DumpJSON(obj)
			},
		},
	}

	luaManager := &LuaManager{}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			fmt.Println(cs.name)
			l, err := luaManager.RunLuaScript(cs.getObj(), cs.getLuaScript())
			if err != nil {
				t.Fatalf("RunLuaScript failed: %s", err.Error())
				return
			}
			returnValue := l.Get(-1)
			if returnValue.Type() != lua.LTTable {
				t.Fatalf("expect table output from Lua script, not %s", returnValue.Type().String())
			}
			jsonBytes, err := luajson.Encode(returnValue)
			if err != nil {
				t.Fatalf("encode failed: %s", err.Error())
				return
			}
			newObj := map[string]string{}
			err = json.Unmarshal(jsonBytes, &newObj)
			if err != nil {
				t.Fatalf("Unmarshal failed: %s", err.Error())
				return
			}
			if util.DumpJSON(newObj) != cs.expectResult() {
				t.Fatalf("expect(%s), but get (%s)", cs.expectResult(), util.DumpJSON(newObj))
			}
		})
	}
}
