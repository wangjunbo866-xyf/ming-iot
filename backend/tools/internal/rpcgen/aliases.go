package rpcgen

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"ming-iot/backend/tools/internal/clientcontract"
)

// computeMissingAliases 比较每个 client package 的：
//   - live/golden client contract 中已有的 alias 名集合
//   - 当前 goctl 生成的 *.go 中已有的公开 type alias 名集合
//
// 返回每个 packageDir 需要补齐的 alias 名（desired 有但 goctl 没生成的）。
func computeMissingAliases(m *Manifest, candidate string, desired clientcontract.Document) (map[string][]string, error) {
	desiredByPkg := map[string]map[string]bool{}
	for _, a := range desired.Aliases {
		if desiredByPkg[a.Package] == nil {
			desiredByPkg[a.Package] = map[string]bool{}
		}
		desiredByPkg[a.Package][a.Name] = true
	}

	// 提取 candidate 当前 client 目录已有的公开面
	clientRoot := m.ClientRoot(candidate)
	existing, err := clientcontract.ExtractWithSelector(clientRoot, m.PBSelector)
	if err != nil {
		return nil, fmt.Errorf("extract existing client contract: %w", err)
	}
	existingByPkg := map[string]map[string]bool{}
	for _, a := range existing.Aliases {
		if existingByPkg[a.Package] == nil {
			existingByPkg[a.Package] = map[string]bool{}
		}
		existingByPkg[a.Package][a.Name] = true
	}

	result := map[string][]string{}
	for _, s := range m.Services {
		need := desiredByPkg[s.PackageDir]
		have := existingByPkg[s.PackageDir]
		var missing []string
		for name := range need {
			if !have[name] {
				missing = append(missing, name)
			}
		}
		sort.Strings(missing)
		result[s.PackageDir] = missing
	}
	return result, nil
}

// loadGolden 读取 clientcontract golden JSON。
func loadGolden(path string) (*clientcontract.Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read golden %s: %w", path, err)
	}
	var doc clientcontract.Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal golden: %w", err)
	}
	return &doc, nil
}
