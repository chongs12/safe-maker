package rule

import (
	"container/list"
	"regexp"
)

// AhoCorasickNode AC自动机节点
type AhoCorasickNode struct {
	children     map[rune]*AhoCorasickNode
	fail         *AhoCorasickNode
	isEnd        bool
	pattern      string
	action       string
	group        string
	isRegex      bool
	regexPattern *regexp.Regexp
}

// AhoCorasick AC自动机
type AhoCorasick struct {
	root *AhoCorasickNode
}

// NewAhoCorasick 创建AC自动机
func NewAhoCorasick() *AhoCorasick {
	return &AhoCorasick{
		root: &AhoCorasickNode{
			children: make(map[rune]*AhoCorasickNode),
		},
	}
}

// AddPattern 添加关键词模式
func (ac *AhoCorasick) AddPattern(pattern, action, group string) {
	node := ac.root
	for _, ch := range pattern {
		if node.children[ch] == nil {
			node.children[ch] = &AhoCorasickNode{
				children: make(map[rune]*AhoCorasickNode),
			}
		}
		node = node.children[ch]
	}
	node.isEnd = true
	node.pattern = pattern
	node.action = action
	node.group = group
	node.isRegex = false
}

// AddRegexPattern 添加正则表达式模式
func (ac *AhoCorasick) AddRegexPattern(pattern, action, group string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}

	// 对于正则，我们存储在根节点的特殊列表中
	// 实际生产环境可以优化为提取公共前缀
	node := ac.root
	node.regexPattern = re
	node.pattern = pattern
	node.action = action
	node.group = group
	node.isRegex = true
	node.isEnd = true

	return nil
}

// Build 构建失败指针
func (ac *AhoCorasick) Build() {
	queue := list.New()

	// 第一层节点的失败指针指向root
	for _, child := range ac.root.children {
		child.fail = ac.root
		queue.PushBack(child)
	}

	// BFS构建失败指针
	for queue.Len() > 0 {
		element := queue.Front()
		queue.Remove(element)
		node := element.Value.(*AhoCorasickNode)

		for ch, child := range node.children {
			// 计算child的失败指针
			fail := node.fail
			for fail != nil {
				if next, ok := fail.children[ch]; ok {
					child.fail = next
					break
				}
				fail = fail.fail
			}
			if child.fail == nil {
				child.fail = ac.root
			}
			queue.PushBack(child)
		}
	}
}

// MatchResult 匹配结果
type MatchResult struct {
	Pattern string
	Action  string
	Group   string
	IsRegex bool
}

// Search 搜索文本中的匹配
func (ac *AhoCorasick) Search(text string) *MatchResult {
	node := ac.root

	for _, ch := range text {
		// 沿着失败指针跳转，直到找到匹配或回到根节点
		for node != ac.root && node.children[ch] == nil {
			node = node.fail
		}

		if next, ok := node.children[ch]; ok {
			node = next
		} else {
			node = ac.root
		}

		// 检查当前节点是否是结束节点
		if node.isEnd && !node.isRegex {
			return &MatchResult{
				Pattern: node.pattern,
				Action:  node.action,
				Group:   node.group,
				IsRegex: false,
			}
		}
	}

	return nil
}

// SearchWithRegex 搜索文本（包含正则匹配）
func (ac *AhoCorasick) SearchWithRegex(text string) *MatchResult {
	// 先进行AC自动机匹配
	if result := ac.Search(text); result != nil {
		return result
	}

	// 再进行正则匹配（遍历根节点的正则模式）
	// 实际生产环境可以优化为更高效的存储方式
	if ac.root.isRegex && ac.root.regexPattern != nil {
		if ac.root.regexPattern.MatchString(text) {
			return &MatchResult{
				Pattern: ac.root.pattern,
				Action:  ac.root.action,
				Group:   ac.root.group,
				IsRegex: true,
			}
		}
	}

	return nil
}

// KeywordMatcher 关键词匹配器接口
type KeywordMatcher interface {
	AddKeyword(keyword, action, group string)
	AddRegex(pattern, action, group string) error
	Build()
	Match(text string) *MatchResult
}

// SimpleMatcher 简单遍历实现（保留用于对比测试）
type SimpleMatcher struct {
	keywords []struct {
		keyword string
		action  string
		group   string
	}
	regexes []struct {
		regex  *regexp.Regexp
		action string
		group  string
	}
}

// NewSimpleMatcher 创建简单匹配器
func NewSimpleMatcher() *SimpleMatcher {
	return &SimpleMatcher{}
}

// AddKeyword 添加关键词
func (sm *SimpleMatcher) AddKeyword(keyword, action, group string) {
	sm.keywords = append(sm.keywords, struct {
		keyword string
		action  string
		group   string
	}{keyword, action, group})
}

// AddRegex 添加正则
func (sm *SimpleMatcher) AddRegex(pattern, action, group string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return err
	}
	sm.regexes = append(sm.regexes, struct {
		regex  *regexp.Regexp
		action string
		group  string
	}{re, action, group})
	return nil
}

// Build 构建（简单匹配器无需构建）
func (sm *SimpleMatcher) Build() {}

// Match 匹配
func (sm *SimpleMatcher) Match(text string) *MatchResult {
	for _, kw := range sm.keywords {
		if contains(text, kw.keyword) {
			return &MatchResult{
				Pattern: kw.keyword,
				Action:  kw.action,
				Group:   kw.group,
			}
		}
	}

	for _, re := range sm.regexes {
		if re.regex.MatchString(text) {
			return &MatchResult{
				Pattern: re.regex.String(),
				Action:  re.action,
				Group:   re.group,
				IsRegex: true,
			}
		}
	}

	return nil
}

// contains 检查字符串是否包含子串（不区分大小写）
func contains(text, substr string) bool {
	// 简化的实现，实际可以用更高效的算法
	return len(substr) <= len(text) && containsHelper(text, substr)
}

func containsHelper(text, substr string) bool {
	for i := 0; i <= len(text)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if toLower(text[i+j]) != toLower(substr[j]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func toLower(ch byte) byte {
	if ch >= 'A' && ch <= 'Z' {
		return ch + ('a' - 'A')
	}
	return ch
}
