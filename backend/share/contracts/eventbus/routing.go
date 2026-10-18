// 本文件提供显式消费者身份和精确兼容选择器，不推导业务授权或订阅范围。
package eventbus

import (
	"fmt"
	"strings"
	"unicode"
)

// BroadcastGroup 为稳定实例建立独立广播组；实例身份必须由部署提供并跨重启保留。
func BroadcastGroup(role, instance string) (string, error) {
	if !validToken(role) || !validToken(instance) {
		return "", fmt.Errorf("invalid broadcast identity")
	}
	return "coord." + role + "." + instance + ".v1", nil
}

// CompetingGroup 为显式逻辑订阅者建立竞争组；不同任务必须传入不同身份。
func CompetingGroup(logical string) (string, error) {
	if len(logical) > 128 {
		return "", fmt.Errorf("logical identity exceeds size limit")
	}
	for _, token := range strings.Split(logical, ".") {
		if !validToken(token) {
			return "", fmt.Errorf("invalid logical identity")
		}
	}
	return "jobs." + logical + ".v1", nil
}

// MatchSubject 按 NATS 兼容语义匹配；星号恰好一段，末尾大于号匹配一段或多段。
func MatchSubject(selector, subject string) (bool, error) {
	// pattern 是已核验的选择器各段。
	pattern, err := subjectTokens(selector, true)
	if err != nil {
		return false, err
	}
	// actual 必须为具体 subject，不能带通配符。
	actual, err := subjectTokens(subject, false)
	if err != nil {
		return false, err
	}
	for index, token := range pattern {
		if index >= len(actual) {
			return false, nil
		}
		if token == ">" {
			return true, nil
		}
		if token != "*" && token != actual[index] {
			return false, nil
		}
	}
	return len(pattern) == len(actual), nil
}

// subjectTokens 拒绝空段、部分通配符、非末尾大于号及空白控制字符。
func subjectTokens(value string, selector bool) ([]string, error) {
	if !validText(value, 1024) {
		return nil, fmt.Errorf("invalid subject")
	}
	// tokens 保留分段边界，不能用字符串前缀替代段匹配。
	tokens := strings.Split(value, ".")
	for index, token := range tokens {
		if token == "" {
			return nil, fmt.Errorf("empty subject token")
		}
		for _, char := range token {
			if unicode.IsSpace(char) {
				return nil, fmt.Errorf("subject contains whitespace")
			}
		}
		if selector && token == "*" {
			continue
		}
		if selector && token == ">" && index == len(tokens)-1 {
			continue
		}
		if strings.ContainsAny(token, "*>") {
			return nil, fmt.Errorf("invalid subject wildcard")
		}
	}
	return tokens, nil
}
