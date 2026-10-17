package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

const contentValidationEventType = "portal.content.validate"

var contentSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type contentValidationInput struct {
	Title       string   `json:"title"`
	Slug        string   `json:"slug"`
	ContentType string   `json:"content_type"`
	Tags        []string `json:"tags"`
	Visibility  string   `json:"visibility"`
	Summary     string   `json:"summary"`
	Body        string   `json:"body"`
}

var approvedContentTags = map[string]struct{}{
	"教育": {}, "学前教育": {}, "小学教育": {}, "中学教育": {}, "高等教育": {}, "职业教育": {}, "家庭教育": {}, "教学资源": {},
	"教育人力资源": {}, "事业编": {}, "国企": {}, "省考": {}, "国考": {},
	"旅游": {}, "住宿": {}, "交通": {}, "景点攻略": {}, "旅行规划": {}, "美食体验": {}, "网络": {}, "网络安全": {}, "网络维护": {},
	"网络基础知识": {}, "网络诊断": {}, "云与服务器": {}, "系统": {}, "操作系统": {}, "系统配置": {}, "系统维护": {}, "部署运维": {},
	"故障排查": {}, "编程": {}, "前端开发": {}, "后端开发": {}, "Python": {}, "JavaScript / TypeScript": {}, "数据库": {},
	"API 与集成": {}, "代码实践": {}, "软件工具": {}, "效率工具": {}, "开发工具": {}, "文档工具": {}, "AI 工具": {}, "自动化工具": {},
}

type contentValidationResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}

// validateContentEvent returns a durable validation result. Invalid content is a
// successful event execution with valid=false; only infrastructure/decoding
// failures return an error and enter the processor retry path.
func validateContentEvent(_ context.Context, event EventInput) (json.RawMessage, error) {
	var input contentValidationInput
	decoder := json.NewDecoder(bytes.NewReader(event.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return marshalContentValidationResult(contentValidationResult{Errors: []string{"payload fields are invalid or have unsupported fields"}})
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return marshalContentValidationResult(contentValidationResult{Errors: []string{"payload must contain exactly one JSON object"}})
	}

	result := validateContent(input)
	return marshalContentValidationResult(result)
}

func marshalContentValidationResult(result contentValidationResult) (json.RawMessage, error) {
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode content validation result: %w", err)
	}
	return encoded, nil
}

func validateContent(input contentValidationInput) contentValidationResult {
	var problems []string
	title := strings.TrimSpace(input.Title)
	if title == "" {
		problems = append(problems, "title is required")
	} else if utf8.RuneCountInString(title) > 200 {
		problems = append(problems, "title must be at most 200 characters")
	}

	slug := strings.TrimSpace(input.Slug)
	if slug == "" {
		problems = append(problems, "slug is required")
	} else if len(slug) > 100 || !contentSlugPattern.MatchString(slug) {
		problems = append(problems, "slug must be 1-100 lowercase letters or digits separated by single hyphens")
	}

	switch input.ContentType {
	case "article", "tool", "resource", "project":
	default:
		problems = append(problems, "content_type must be article, tool, resource, or project")
	}
	if len(input.Tags) > 40 {
		problems = append(problems, "tags must contain at most 40 approved tags")
	} else {
		seenTags := make(map[string]struct{}, len(input.Tags))
		for _, tag := range input.Tags {
			if _, ok := approvedContentTags[tag]; !ok {
				problems = append(problems, "tags must contain only approved Knowledge Plaza tags")
				break
			}
			if _, duplicate := seenTags[tag]; duplicate {
				problems = append(problems, "tags must not contain duplicates")
				break
			}
			seenTags[tag] = struct{}{}
		}
	}
	switch input.Visibility {
	case "public", "private":
	default:
		problems = append(problems, "visibility must be public or private")
	}
	if utf8.RuneCountInString(strings.TrimSpace(input.Summary)) > 500 {
		problems = append(problems, "summary must be at most 500 characters")
	}
	if strings.TrimSpace(input.Body) == "" {
		problems = append(problems, "body is required")
	} else if utf8.RuneCountInString(input.Body) > 100000 {
		problems = append(problems, "body must be at most 100000 characters")
	}
	return contentValidationResult{Valid: len(problems) == 0, Errors: problems}
}
