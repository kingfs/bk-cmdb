/*
 * Tencent is pleased to support the open source community by making 蓝鲸 available.
 * Copyright (C) 2017-2018 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */

package instdata

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"configcenter/src/common"
	"configcenter/src/common/base"
	"configcenter/src/common/blog"
	"configcenter/src/common/core/cc/actions"
	meta "configcenter/src/common/metadata"
	"configcenter/src/common/util"
	dcCommon "configcenter/src/scene_server/datacollection/common"
	eventtypes "configcenter/src/scene_server/event_server/types"
	"configcenter/src/source_controller/common/commondata"
	"configcenter/src/source_controller/common/eventdata"
	"configcenter/src/source_controller/common/instdata"
	"github.com/emicklei/go-restful"
)

func init() {
	actions.RegisterNewAction(actions.Action{Verb: common.HTTPSelectGet, Path: "/host/{bk_host_id}", Params: nil, Handler: host.GetHostByID})
	actions.RegisterNewAction(actions.Action{Verb: common.HTTPSelectPost, Path: "/hosts/search", Params: nil, Handler: host.GetHosts})
	actions.RegisterNewAction(actions.Action{Verb: common.HTTPCreate, Path: "/insts", Params: nil, Handler: host.AddHost})
	actions.RegisterNewAction(actions.Action{Verb: common.HTTPSelectGet, Path: "/host/snapshot/{bk_host_id}", Params: nil, Handler: host.GetHostSnap})
	host.CreateAction()
}

var host *hostAction = &hostAction{}

type hostAction struct {
	base.BaseAction
}

//AddHost add host to resource
func (cli *hostAction) AddHost(req *restful.Request, resp *restful.Response) {
	language := util.GetLanguage(req.Request.Header)
	defErr := cli.CC.Error.CreateDefaultCCErrorIf(language)

	objType := common.BKInnerObjIDHost
	instdata.DataH = cli.CC.InstCli

	input := make(map[string]interface{})
	if err := json.NewDecoder(req.Request.Body).Decode(input); err != nil {
		blog.Errorf("add host failed with decode body err: %v", err)
		resp.WriteError(http.StatusBadRequest, &meta.RespError{Msg: defErr.Error(common.CCErrCommJSONUnmarshalFailed)})
		return
	}

	input[common.CreateTimeField] = time.Now()
	var idName string
	ID, err := instdata.CreateObject(objType, input, &idName)
	if err != nil {
		blog.Errorf("create object type:%s ,data: %v error: %v", objType, input, err)
		resp.WriteError(http.StatusBadRequest, &meta.RespError{Msg: defErr.Error(common.CCErrHostCreateInst)})
		return
	}

	// record event
	originData := map[string]interface{}{}
	if err := instdata.GetObjectByID(objType, nil, ID, originData, ""); err != nil {
		blog.Error("create event error:%v", err)
	} else {
		ec := eventdata.NewEventContextByReq(req)
		err := ec.InsertEvent(eventtypes.EventTypeInstData, "host", eventtypes.EventActionCreate, originData, nil)
		if err != nil {
			blog.Errorf("add host success, but create event failed, err: %v", err)
		}
	}

	resp.WriteEntity(meta.NewSuccessResp(map[string]int{idName: ID}))
}

//GetHostByID get host detail
func (cli *hostAction) GetHostByID(req *restful.Request, resp *restful.Response) {
	language := util.GetActionLanguage(req)
	defErr := cli.CC.Error.CreateDefaultCCErrorIf(language)

	pathParams := req.PathParameters()
	hostID, err := strconv.Atoi(pathParams["bk_host_id"])
	if err != nil {
		resp.WriteError(http.StatusBadRequest, &meta.RespError{Msg: defErr.Error(common.CCErrCommParamsIsInvalid)})
		return
	}

	var result interface{}
	condition := make(map[string]interface{})
	condition[common.BKHostIDField] = hostID
	fields := make([]string, 0)
	err = cli.CC.InstCli.GetOneByCondition("cc_HostBase", fields, condition, &result)
	if err != nil {
		blog.Error("get host by id failed, err: %v", err)
		resp.WriteError(http.StatusBadRequest, &meta.RespError{Msg: defErr.Error(common.CCErrCommDBSelectFailed)})
		return
	}

	resp.WriteEntity(meta.NewSuccessResp(result))
}

//GetHosts batch search host
func (cli *hostAction) GetHosts(req *restful.Request, resp *restful.Response) {
	language := util.GetActionLanguage(req)
	defErr := cli.CC.Error.CreateDefaultCCErrorIf(language)
	defLang := cli.CC.Lang.CreateDefaultCCLanguageIf(language)

	objType := common.BKInnerObjIDHost
	instdata.DataH = cli.CC.InstCli

	var dat commondata.ObjQueryInput
	if err := json.NewDecoder(req.Request.Body).Decode(&dat); err != nil {
		blog.Errorf("get host failed with decode body err: %v", err)
		resp.WriteError(http.StatusBadRequest, &meta.RespError{Msg: defErr.Error(common.CCErrCommJSONUnmarshalFailed)})
		return
	}

	condition := util.ConvParamsTime(dat.Condition)
	fieldArr := strings.Split(dat.Fields, ",")
	result := make([]map[string]interface{}, 0)

	err := instdata.GetObjectByCondition(defLang, objType, fieldArr, condition, &result, dat.Sort, dat.Start, dat.Limit)
	if err != nil {
		blog.Error("get object failed type:%s,input:%v error:%v", objType, dat, err)
		resp.WriteError(http.StatusBadRequest, &meta.RespError{Msg: defErr.Error(common.CCErrHostSelectInst)})
		return
	}

	count, err := instdata.GetCntByCondition(objType, condition)
	if err != nil {
		blog.Error("get object failed type:%s ,input: %v error: %v", objType, dat, err)
		resp.WriteError(http.StatusBadRequest, &meta.RespError{Msg: defErr.Error(common.CCErrHostSelectInst)})
		return
	}

	resp.WriteEntity(meta.GetHostsResult{
		BaseResp: meta.SuccessBaseResp,
		Data:     meta.HostInfo{Count: count, Info: result},
	})
}

//GetHostSnap get host snap
func (cli *hostAction) GetHostSnap(req *restful.Request, resp *restful.Response) {
	language := util.GetActionLanguage(req)
	defErr := cli.CC.Error.CreateDefaultCCErrorIf(language)

	hostID := req.PathParameter("bk_host_id")
	data := common.KvMap{"key": dcCommon.RedisSnapKeyPrefix + hostID}
	result := ""
	err := cli.CC.CacheCli.GetOneByCondition("Get", nil, data, &result)
	if err != nil {
		blog.Error("get host snapshot failed, hostid: %v, err: %v ", hostID, err)
		resp.WriteError(http.StatusBadRequest, &meta.RespError{Msg: defErr.Error(common.CCErrHostGetSnapshot)})
		return
	}

	resp.WriteEntity(meta.GetHostSnapResult{
		BaseResp: meta.SuccessBaseResp,
		Data:     meta.HostSnap{Data: result},
	})
}
