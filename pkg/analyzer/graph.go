package analyzer

import (
	"strings"

	"github.com/prunedocker/prunedocker/pkg/docker"
)

// LayerNode represents a unique layer in the Docker storage graph.
type LayerNode struct {
	Hash             string   `json:"hash"`
	ShortHash        string   `json:"short_hash"`
	RefCount         int      `json:"ref_count"`
	AssociatedImages []string `json:"associated_images"`
	ParentHash       string   `json:"parent_hash,omitempty"`
	ChildrenHashes   []string `json:"children_hashes,omitempty"`
}

// ImageNode represents an image vertex in the dependency graph.
type ImageNode struct {
	Metadata       docker.ImageMetadata `json:"metadata"`
	IsLeaf         bool                 `json:"is_leaf"`
	DependentCount int                  `json:"dependent_count"`
	LayerChain     []string             `json:"layer_chain"`
}

// DependencyDAG models the Directed Acyclic Graph of all image layers and image dependencies.
type DependencyDAG struct {
	LayerMap    map[string]*LayerNode `json:"layer_map"`
	ImageMap    map[string]*ImageNode `json:"image_map"`
	RootLayers  []string              `json:"root_layers"`
	LeafImages  []string              `json:"leaf_images"`
	TotalLayers int                   `json:"total_layers"`
	TotalImages int                   `json:"total_images"`
}

// BuildDAG constructs the Layer and Image Dependency DAG from an engine snapshot.
func BuildDAG(snapshot *docker.EngineSnapshot) *DependencyDAG {
	dag := &DependencyDAG{
		LayerMap: make(map[string]*LayerNode),
		ImageMap: make(map[string]*ImageNode),
	}

	if snapshot == nil {
		return dag
	}

	// 1. Register all layers and their occurrences across images
	for _, img := range snapshot.Images {
		imgNode := &ImageNode{
			Metadata:   img,
			IsLeaf:     true, // initial assumption, adjusted below
			LayerChain: img.Layers,
		}
		dag.ImageMap[img.ID] = imgNode

		var prevLayerHash string
		for _, layerHash := range img.Layers {
			node, exists := dag.LayerMap[layerHash]
			if !exists {
				node = &LayerNode{
					Hash:             layerHash,
					ShortHash:        docker.CleanID(layerHash),
					RefCount:         0,
					AssociatedImages: make([]string, 0),
					ParentHash:       prevLayerHash,
					ChildrenHashes:   make([]string, 0),
				}
				dag.LayerMap[layerHash] = node
				if prevLayerHash == "" {
					dag.RootLayers = append(dag.RootLayers, layerHash)
				}
			}

			// Link parent to child if not already linked
			if prevLayerHash != "" {
				parentNode := dag.LayerMap[prevLayerHash]
				if parentNode != nil && !containsString(parentNode.ChildrenHashes, layerHash) {
					parentNode.ChildrenHashes = append(parentNode.ChildrenHashes, layerHash)
				}
			}

			node.RefCount++
			if !containsString(node.AssociatedImages, img.ID) {
				node.AssociatedImages = append(node.AssociatedImages, img.ID)
			}

			prevLayerHash = layerHash
		}
	}

	// 2. Identify leaf images and parent-child image dependencies
	// An image A is NOT a leaf if there is an image B such that A's layer chain is a strict prefix of B's layer chain,
	// or B has A as ParentID.
	for idA, nodeA := range dag.ImageMap {
		for idB, nodeB := range dag.ImageMap {
			if idA == idB {
				continue
			}

			// Check explicit ParentID
			if nodeB.Metadata.ParentID != "" && (nodeB.Metadata.ParentID == idA || nodeB.Metadata.ParentID == nodeA.Metadata.ShortID) {
				nodeA.IsLeaf = false
				nodeA.DependentCount++
				continue
			}

			// Check layer prefix relationship
			lenA := len(nodeA.LayerChain)
			lenB := len(nodeB.LayerChain)
			if lenA > 0 && lenA < lenB {
				isPrefix := true
				for i := 0; i < lenA; i++ {
					if nodeA.LayerChain[i] != nodeB.LayerChain[i] {
						isPrefix = false
						break
					}
				}
				if isPrefix {
					nodeA.IsLeaf = false
					nodeA.DependentCount++
				}
			}
		}

		if nodeA.IsLeaf {
			dag.LeafImages = append(dag.LeafImages, idA)
		}
	}

	dag.TotalLayers = len(dag.LayerMap)
	dag.TotalImages = len(dag.ImageMap)

	return dag
}

// GetMaxReuseLayer returns the layer with the highest reference count in the DAG.
func (d *DependencyDAG) GetMaxReuseLayer() *LayerNode {
	var maxNode *LayerNode
	maxCount := -1
	for _, node := range d.LayerMap {
		if node.RefCount > maxCount {
			maxCount = node.RefCount
			maxNode = node
		}
	}
	return maxNode
}

// GetSharedPrefixLength returns how many initial layers are shared between two images.
func (d *DependencyDAG) GetSharedPrefixLength(imgA, imgB string) int {
	nodeA, okA := d.ImageMap[imgA]
	nodeB, okB := d.ImageMap[imgB]
	if !okA || !okB {
		return 0
	}

	minLen := len(nodeA.LayerChain)
	if len(nodeB.LayerChain) < minLen {
		minLen = len(nodeB.LayerChain)
	}

	shared := 0
	for i := 0; i < minLen; i++ {
		if nodeA.LayerChain[i] == nodeB.LayerChain[i] {
			shared++
		} else {
			break
		}
	}
	return shared
}

func containsString(slice []string, val string) bool {
	for _, item := range slice {
		if strings.EqualFold(item, val) {
			return true
		}
	}
	return false
}
