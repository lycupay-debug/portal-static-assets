package main

import "strings"

func validateContentInput(in portalContentInput, partial bool) (map[string]any, []string) {
	v := map[string]any{}
	errs := []string{}
	setString := func(key string, p *string, maxLen int, required bool) {
		if p == nil {
			if required && !partial {
				errs = append(errs, key+"为必填项。")
			}
			return
		}
		s := strings.TrimSpace(*p)
		if required && s == "" {
			errs = append(errs, key+"为必填项。")
		}
		if runeLen(s) > maxLen {
			errs = append(errs, key+"长度超出限制。")
		}
		v[key] = s
	}
	setString("title", in.Title, 200, true)
	setString("slug", in.Slug, 100, true)
	if s, ok := v["slug"].(string); ok && (s == "" || !portalSlug.MatchString(s)) {
		errs = append(errs, "别名必须由小写字母、数字和单个连字符组成。")
	}
	setString("contentType", in.ContentType, 20, true)
	if s, ok := v["contentType"].(string); ok && !portalTypes[s] {
		errs = append(errs, "内容类型不正确。")
	}
	if in.Tags == nil {
		if !partial {
			errs = append(errs, "请至少选择一个知识广场标签。")
			v["tags"] = []string{}
		}
	} else {
		tags := append([]string{}, (*in.Tags)...)
		if len(tags) > 40 {
			errs = append(errs, "标签最多 40 个。")
		}
		seen := map[string]bool{}
		for _, t := range tags {
			if !portalTags[t] || seen[t] {
				errs = append(errs, "知识广场标签不正确，请从现有标签中选择。")
				break
			}
			seen[t] = true
		}
		v["tags"] = tags
	}
	setString("summary", in.Summary, 500, false)
	if _, ok := v["summary"]; !ok && !partial {
		v["summary"] = ""
	}
	setString("body", in.Body, 100000, true)
	setString("visibility", in.Visibility, 20, false)
	if s, ok := v["visibility"].(string); ok && s != "public" && s != "private" {
		errs = append(errs, "可见范围不正确。")
	} else if !ok && !partial {
		v["visibility"] = "private"
	}
	if in.ParentCategorySet || in.ParentCategory != nil {
		if in.ParentCategory == nil {
			v["parentCategory"] = nil
		} else {
			s := strings.TrimSpace(*in.ParentCategory)
			if s == "" {
				v["parentCategory"] = nil
			} else {
				v["parentCategory"] = s
			}
		}
	}
	if in.ChildCategorySet || in.ChildCategory != nil {
		if in.ChildCategory == nil {
			v["childCategory"] = nil
		} else {
			s := strings.TrimSpace(*in.ChildCategory)
			if s == "" {
				v["childCategory"] = nil
			} else {
				v["childCategory"] = s
			}
		}
	}
	p, pok := v["parentCategory"].(string)
	c, cok := v["childCategory"].(string)
	taxonomyPairSupplied := (!partial && (pok || cok)) || (in.ParentCategorySet && in.ChildCategorySet)
	if taxonomyPairSupplied {
		if (pok != cok) || (pok && (p == "" || !hasKey(portalTaxonomy, p))) || (cok && (c == "" || !contains(portalTaxonomy[p], c))) {
			errs = append(errs, "子类必须属于当前选择的父类，且父类和子类必须同时填写。")
		}
	}
	if pok && !hasKey(portalTaxonomy, p) {
		errs = append(errs, "父类分类不正确。")
	}
	return v, errs
}
func hasKey(m map[string][]string, k string) bool { _, ok := m[k]; return ok }
