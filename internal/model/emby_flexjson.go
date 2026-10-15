package model

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Emby 兼容层的宽容 JSON 标量类型。
//
// 背景：Emby 协议事实标准（Jellyfin）在 API 层全局开启了
// System.Text.Json 的 NumberHandling = AllowReadingFromString，并额外注册
// JsonStringConverter（非字符串 token 也按字符串读取）。因此真实客户端会
// 发出「数字写成字符串」的 body，例如 DeviceProfile.TranscodingProfiles[0]
// .MinSegments = "1"。参考 Jellyfin src/Jellyfin.Extensions/Json/JsonDefaults.cs
// 与 Converters/JsonStringConverter.cs。
//
// 本包用 FlexInt / FlexInt64 / FlexBool 在 Go 侧提供等价语义：
//   - 数字、字符串形式的数字、带引号或空白的值都能解析
//   - null、空串、无法解析的值一律取零值且不返回错误
//
// 关键约束：UnmarshalJSON 永不返回非 nil 错误。播放接口的准入门槛是
// 「令牌有效 + 媒体可见」，任何请求体字段都不应导致整个播放请求被拒绝。

// flexParseInt64 尽力把 JSON 标量解析为 int64。
// 第二个返回值表示是否成功解析出有效数值。
func flexParseInt64(raw []byte) (int64, bool) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return 0, false
	}
	// 去掉字符串引号；同时容忍客户端偶发的单引号写法。
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			s = strings.TrimSpace(s[1 : len(s)-1])
			if s == "" {
				return 0, false
			}
		}
	}
	// 布尔字面量按 1/0 处理，与 .NET 数值转换的宽松度对齐。
	switch strings.ToLower(s) {
	case "true":
		return 1, true
	case "false":
		return 0, true
	}
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		return v, true
	}
	// 客户端可能发浮点形式（如 1.0），取整数部分。
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(f), true
	}
	return 0, false
}

// flexParseBool 尽力把 JSON 标量解析为 bool。
func flexParseBool(raw []byte) (bool, int64, bool) {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return false, 0, false
	}
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			s = strings.TrimSpace(s[1 : len(s)-1])
			if s == "" {
				return false, 0, false
			}
		}
	}
	if b, err := strconv.ParseBool(strings.ToLower(s)); err == nil {
		return b, 0, true
	}
	if v, ok := flexParseInt64([]byte(s)); ok {
		return v != 0, v, true
	}
	return false, 0, false
}

// FlexInt 是可从数字、字符串或 null 解析的 int。
// 解析失败时取零值且不报错。
type FlexInt int

// UnmarshalJSON 实现宽容反序列化；永不返回非 nil 错误。
func (f *FlexInt) UnmarshalJSON(data []byte) error {
	if v, ok := flexParseInt64(data); ok {
		*f = FlexInt(v)
		return nil
	}
	*f = 0
	return nil
}

// MarshalJSON 以数字形式写出。
func (f FlexInt) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatInt(int64(f), 10)), nil
}

// Int 返回底层 int 值。
func (f FlexInt) Int() int { return int(f) }

// FlexInt64 是可从数字、字符串或 null 解析的 int64。
// 解析失败时取零值且不报错。
type FlexInt64 int64

// UnmarshalJSON 实现宽容反序列化；永不返回非 nil 错误。
func (f *FlexInt64) UnmarshalJSON(data []byte) error {
	if v, ok := flexParseInt64(data); ok {
		*f = FlexInt64(v)
		return nil
	}
	*f = 0
	return nil
}

// MarshalJSON 以数字形式写出。
func (f FlexInt64) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatInt(int64(f), 10)), nil
}

// Int64 返回底层 int64 值。
func (f FlexInt64) Int64() int64 { return int64(f) }

// FlexBool 是可从布尔、数字、字符串或 null 解析的 bool。
// 解析失败时取 false 且不报错。
type FlexBool bool

// UnmarshalJSON 实现宽容反序列化；永不返回非 nil 错误。
func (f *FlexBool) UnmarshalJSON(data []byte) error {
	if b, _, ok := flexParseBool(data); ok {
		*f = FlexBool(b)
		return nil
	}
	*f = false
	return nil
}

// MarshalJSON 以布尔形式写出。
func (f FlexBool) MarshalJSON() ([]byte, error) {
	if f {
		return []byte("true"), nil
	}
	return []byte("false"), nil
}

// Bool 返回底层 bool 值。
func (f FlexBool) Bool() bool { return bool(f) }

// EmbyDeviceProfile 需要容忍「对象」和「数组」两种形态：官方契约把
// PlaybackInfoRequest.DeviceProfile 标注为 DeviceProfile[]，而绝大多数实现
// 发送单个对象。两种都必须接受，其余形态退化为 nil 而不报错。
func (d *EmbyDeviceProfile) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*d = EmbyDeviceProfile{}
		return nil
	}
	// 先按对象解析（绝大多数客户端的实际形态）。
	if strings.HasPrefix(trimmed, "{") {
		type alias EmbyDeviceProfile
		var obj alias
		if err := json.Unmarshal(data, &obj); err != nil {
			// 对象内部有非法字段时，整体退化为空 profile 而非拒绝请求。
			*d = EmbyDeviceProfile{}
			return nil
		}
		*d = EmbyDeviceProfile(obj)
		return nil
	}
	// 再按数组解析，取首个元素（官方契约的 DeviceProfile[] 形态）。
	if strings.HasPrefix(trimmed, "[") {
		type alias EmbyDeviceProfile
		var arr []alias
		if err := json.Unmarshal(data, &arr); err != nil || len(arr) == 0 {
			*d = EmbyDeviceProfile{}
			return nil
		}
		*d = EmbyDeviceProfile(arr[0])
		return nil
	}
	// 其他形态（字符串、数字等）一律退化为空 profile。
	*d = EmbyDeviceProfile{}
	return nil
}
