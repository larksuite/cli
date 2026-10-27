// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

// isMultiSourceDataConfig 判断 data_config 是否走多数据源协议。
func isMultiSourceDataConfig(cfg map[string]interface{}) bool {
	_, ok := cfg["data_sources"]
	return ok
}

// normalizeMultiSourceDataConfig 归一化多源配置：series rollup 大写。
// 仅做安全归一化，不补默认值；校验统一由 validateMultiSourceBlockDataConfig 处理。
func normalizeMultiSourceDataConfig(cfg map[string]interface{}) map[string]interface{} {
	out := cloneMap(cfg)
	if out == nil {
		return map[string]interface{}{}
	}
	rawSources, ok := out["data_sources"].([]interface{})
	if !ok {
		return out
	}
	for i, raw := range rawSources {
		source, isMap := raw.(map[string]interface{})
		if !isMap {
			continue
		}
		rawSources[i] = normalizeDataConfig(source)
	}
	out["data_sources"] = rawSources
	return out
}
