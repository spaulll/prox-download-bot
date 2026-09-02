package organize

import (
	"path/filepath"
	"sort"
	"strings"
)

type treeNode struct {
	children map[string]*treeNode
	files    []string
}

func newTreeNode() *treeNode {
	return &treeNode{children: map[string]*treeNode{}}
}

// Tree renders the library-relative directory structure of moved files using
// the plan-mandated tree style:
//
//	Series
//	└── Some Show
//	    └── Season 2
//	        └── Some.Show.S02E04.mkv
//
// libraryRoots maps a category display label to its absolute base dir.
func Tree(paths []string, libraryRoots map[string]string) string {
	root := newTreeNode()

	for _, p := range paths {
		abs := filepath.Clean(p)
		rel := ""
		catLabel := "Others"
		for label, dir := range libraryRoots {
			if dir == "" {
				continue
			}
			dirAbs := filepath.Clean(dir) + string(filepath.Separator)
			if strings.HasPrefix(abs+string(filepath.Separator), dirAbs) {
				rel = strings.TrimPrefix(abs, filepath.Clean(dir))
				catLabel = label
				break
			}
		}
		if rel == "" {
			rel = filepath.Base(abs)
		}
		rel = strings.TrimPrefix(rel, string(filepath.Separator))
		parts := strings.Split(rel, string(filepath.Separator))

		catNode, ok := root.children[catLabel]
		if !ok {
			catNode = newTreeNode()
			root.children[catLabel] = catNode
		}
		cur := catNode
		for i, part := range parts {
			if i == len(parts)-1 {
				cur.files = append(cur.files, part)
				break
			}
			next, ok := cur.children[part]
			if !ok {
				next = newTreeNode()
				cur.children[part] = next
			}
			cur = next
		}
	}

	var b strings.Builder
	renderNode(&b, root, "", 0)
	return strings.TrimRight(b.String(), "\n")
}

// renderNode prints a node: dirs sorted, then files sorted. Direct children
// of the root get no tree prefix; deeper levels get ├── / └── with │
// continuation lines.
func renderNode(b *strings.Builder, n *treeNode, prefix string, depth int) {
	names := make([]string, 0, len(n.children))
	for k := range n.children {
		names = append(names, k)
	}
	sort.Strings(names)
	entries := len(names) + len(n.files)

	idx := 0
	for _, name := range names {
		last := idx == entries-1
		renderEntry(b, n.children[name], name, prefix, depth, last, true)
		idx++
	}
	sort.Strings(n.files)
	for _, f := range n.files {
		last := idx == entries-1
		if depth == 0 {
			b.WriteString(f + "\n")
		} else if last {
			b.WriteString(prefix + "└── " + f + "\n")
		} else {
			b.WriteString(prefix + "├── " + f + "\n")
		}
		idx++
	}
}

func renderEntry(b *strings.Builder, child *treeNode, name, prefix string, depth int, last, isDir bool) {
	if depth == 0 {
		b.WriteString(name + "\n")
	} else if last {
		b.WriteString(prefix + "└── " + name + "\n")
	} else {
		b.WriteString(prefix + "├── " + name + "\n")
	}
	childPrefix := prefix
	if depth > 0 {
		if last {
			childPrefix = prefix + "    "
		} else {
			childPrefix = prefix + "│   "
		}
	}
	renderNode(b, child, childPrefix, depth+1)
}
