/*
 * Tencent is pleased to support the open source community by making 蓝鲸 available.
 * Copyright (C) 2017-2019 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */

package logics

import (
	"strconv"

	"configcenter/src/ac/iam"
	"configcenter/src/common"
	"configcenter/src/common/blog"
	"configcenter/src/common/http/rest"
	"configcenter/src/common/metadata"
	"configcenter/src/common/util"
	sdktypes "configcenter/src/scene_server/auth_server/sdk/types"
	"configcenter/src/scene_server/auth_server/types"
)

var resourceParentMap = iam.GetResourceParentMap()

// fetch resource instances' specified attributes info using instance ids
func (lgc *Logics) FetchInstanceInfo(kit *rest.Kit, resourceType iam.TypeID, filter *types.FetchInstanceInfoFilter,
	page types.Page, extraCond map[string]interface{}) ([]map[string]interface{}, error) {

	idField := GetResourceIDField(resourceType)
	nameField := GetResourceNameField(resourceType)
	if idField == "" || nameField == "" {
		blog.Errorf("request type %s is invalid, rid: %s", resourceType, kit.Rid)
		return nil, kit.CCError.CCErrorf(common.CCErrCommParamsIsInvalid, "type")
	}

	if len(filter.Attrs) == 0 {
		return make([]map[string]interface{}, 0), nil
	}

	// if attribute filter is set, add id attribute and convert display_name to the real name field
	var attrs []string
	needPath := false
	if len(filter.Attrs) > 0 {
		attrs = append(filter.Attrs, idField)
		for index, attr := range attrs {
			if attr == types.NameField {
				attrs[index] = nameField
				break
			}
			if attr == sdktypes.IamPathKey {
				needPath = true
			}
		}
		if needPath && resourceType != iam.Host {
			for _, parent := range resourceParentMap[resourceType] {
				attrs = append(attrs, string(parent))
			}
		}
	}

	cond := make(map[string]interface{})
	if isResourceIDStringType(resourceType) {
		cond[idField] = map[string]interface{}{
			common.BKDBIN: filter.IDs,
		}
	} else {
		ids := make([]int64, len(filter.IDs))
		for idx, idStr := range filter.IDs {
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				blog.Errorf("id %s parse int failed, error: %s, rid: %s, skip it", idStr, err.Error(), kit.Rid)
				continue
			}
			ids[idx] = id
		}
		cond[idField] = map[string]interface{}{
			common.BKDBIN: ids,
		}
	}

	if len(extraCond) > 0 {
		cond = map[string]interface{}{
			common.BKDBAND: []map[string]interface{}{cond, extraCond},
		}
	}

	param := metadata.PullResourceParam{
		Condition: cond,
		Fields:    attrs,
		Limit:     page.Limit,
		Offset:    page.Offset,
	}
	instances, err := lgc.searchAuthResource(kit, param, resourceType)
	if err != nil {
		blog.ErrorJSON("search auth resource failed, error: %s, param: %s, rid: %s", err.Error(), param, kit.Rid)
		return nil, err
	}

	// covert id and display_name field
	for _, instance := range instances.Info {
		instance[types.IDField] = util.GetStrByInterface(instance[idField])
		if instance[nameField] != nil {
			instance[types.NameField] = util.GetStrByInterface(instance[nameField])
		}
		if needPath {
			instance[sdktypes.IamPathKey], err = lgc.getResourceIamPath(kit, resourceType, instance)
			if err != nil {
				blog.ErrorJSON("getResourceIamPath failed, error: %s, instance: %s, rid: %s", err.Error(), instance, kit.Rid)
				return nil, err
			}
		}
	}
	return instances.Info, nil
}

func (lgc *Logics) ValidateFetchInstanceInfoRequest(kit *rest.Kit, req *types.PullResourceReq) (*types.FetchInstanceInfoFilter, error) {
	filter, ok := req.Filter.(types.FetchInstanceInfoFilter)
	if !ok {
		blog.ErrorJSON("request filter %s is not the right type for fetch_instance_info method, rid: %s", req.Filter, kit.Rid)
		return nil, kit.CCError.CCErrorf(common.CCErrCommParamsIsInvalid, "filter")
	}

	if len(filter.IDs) == 0 {
		blog.ErrorJSON("request filter %s ids not set for fetch_instance_info method, rid: %s", req.Filter, kit.Rid)
		return nil, kit.CCError.CCErrorf(common.CCErrCommParamsNeedSet, "filter.ids")
	}

	if req.Page.IsIllegal() {
		blog.Errorf("request page limit %d exceeds max page size, rid: %s", req.Page.Limit, kit.Rid)
		return nil, kit.CCError.CCErrorf(common.CCErrCommPageLimitIsExceeded)
	}
	return &filter, nil
}

// get resource iam path
func (lgc *Logics) getResourceIamPath(kit *rest.Kit, resourceType iam.TypeID, instance map[string]interface{}) ([]string, error) {
	iamPath := make([]string, 0)
	if resourceType != iam.Host {
		// currently all resources only have one layer TODO support multiple layers if needed
		for _, parent := range resourceParentMap[resourceType] {
			iamPath = append(iamPath, "/"+string(parent)+","+util.GetStrByInterface(instance[GetResourceIDField(parent)])+"/")
		}
		return iamPath, nil
	}

	hostID, err := util.GetInt64ByInterface(instance[common.BKHostIDField])
	if err != nil {
		blog.Errorf("hostID %v parse int failed, error: %s, rid: %s", instance[common.BKHostIDField], err.Error(), kit.Rid)
		return nil, err
	}

	// get host iam path, either in resource pool directory or in business TODO: support host in business module when topology is supported
	defaultBizID, err := lgc.GetResourcePoolBizID(kit)
	if err != nil {
		return nil, err
	}

	req := &metadata.HostModuleRelationRequest{
		HostIDArr: []int64{hostID},
		Fields:    []string{common.BKHostIDField, common.BKAppIDField, common.BKSetIDField, common.BKModuleIDField},
		Page: metadata.BasePage{
			Limit: common.BKNoLimit,
		},
	}
	res, err := lgc.CoreAPI.CoreService().Host().GetHostModuleRelation(kit.Ctx, kit.Header, req)
	if err != nil {
		blog.Errorf("GetHostModuleRelation by host id %d failed, error: %s, rid: %s", hostID, err.Error(), kit.Rid)
		return nil, err
	}
	if !res.Result {
		blog.Errorf("GetHostModuleRelation by host id %d failed, error code: %d, error message: %s, rid: %s", hostID, res.Code, res.ErrMsg, kit.Rid)
		return nil, res.CCError()
	}
	if len(res.Data.Info) == 0 {
		return nil, nil
	}

	relationDistinctMap := make(map[string]bool)
	for _, relation := range res.Data.Info {
		var path string
		if relation.AppID == defaultBizID {
			path = "/" + string(iam.SysResourcePoolDirectory) + "," + strconv.FormatInt(relation.ModuleID, 10) + "/"
		} else {
			iamPath = append(iamPath, "/"+string(iam.Business)+","+strconv.FormatInt(relation.AppID, 10)+"/")
		}
		if !relationDistinctMap[path] {
			relationDistinctMap[path] = true
			iamPath = append(iamPath, path)
		}
	}
	return iamPath, nil
}