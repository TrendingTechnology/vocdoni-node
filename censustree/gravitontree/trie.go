// Package tree provides the functions for creating and managing an iden3 merkletree
package gravitontree

import (
	"fmt"
	"path"
	"sync/atomic"
	"time"

	"git.sr.ht/~sircmpwn/go-bare"
	"go.vocdoni.io/dvote/censustree"
	"go.vocdoni.io/dvote/log"
	"go.vocdoni.io/dvote/statedb"
	"go.vocdoni.io/dvote/statedb/gravitonstate"
)

type Tree struct {
	Tree           statedb.StateTree
	store          statedb.StateDB
	dataDir        string
	name           string
	public         uint32
	lastAccessUnix int64 // a unix timestamp, used via sync/atomic
}

type exportElement struct {
	Key   []byte `bare:"key"`
	Value []byte `bare:"value"`
}

type exportData struct {
	Elements []exportElement `bare:"elements"`
}

const (
	MaxKeySize   = 128
	MaxValueSize = 256
)

// NewTree opens or creates a merkle tree under the given storage.
// Note that each tree should use an entirely separate namespace for its database keys.
func NewTree(name, storageDir string) (censustree.Tree, error) {
	gs := new(gravitonstate.GravitonState)
	iname := name
	if len(iname) > 32 {
		iname = iname[:32]
	}
	dir := fmt.Sprintf("%s/%s", storageDir, name)
	log.Debugf("creating census tree %s on %s", iname, dir)

	if err := gs.Init(dir, "disk"); err != nil {
		return nil, err
	}
	if err := gs.AddTree(iname); err != nil {
		return nil, err
	}
	if err := gs.LoadVersion(0); err != nil {
		return nil, err
	}
	tr := &Tree{store: gs, Tree: gs.Tree(iname), name: iname, dataDir: dir}
	tr.updateAccessTime()
	return tr, nil
}

func (t *Tree) Init(name, storageDir string) error {
	gs := new(gravitonstate.GravitonState)
	iname := name
	if len(iname) > 32 {
		iname = iname[:32]
	}
	dir := path.Join(storageDir, name)
	log.Debugf("creating census tree %s on %s", iname, dir)

	if err := gs.Init(dir, "disk"); err != nil {
		return err
	}
	if err := gs.AddTree(iname); err != nil {
		return err
	}
	if err := gs.LoadVersion(0); err != nil {
		return err
	}
	t.store = gs
	t.Tree = gs.Tree(iname)
	t.name = iname
	t.dataDir = dir
	t.updateAccessTime()
	return nil
}

func (t *Tree) MaxKeySize() int {
	return MaxKeySize
}

// LastAccess returns the last time the Tree was accessed, in the form of a unix
// timestamp.
func (t *Tree) LastAccess() int64 {
	return atomic.LoadInt64(&t.lastAccessUnix)
}

func (t *Tree) updateAccessTime() {
	atomic.StoreInt64(&t.lastAccessUnix, time.Now().Unix())
}

// Publish makes a merkle tree available for queries.
// Application layer should check IsPublish() before considering the Tree available.
func (t *Tree) Publish() {
	atomic.StoreUint32(&t.public, 1)
}

// UnPublish makes a merkle tree not available for queries
func (t *Tree) UnPublish() {
	atomic.StoreUint32(&t.public, 0)
}

// IsPublic returns true if the tree is available
func (t *Tree) IsPublic() bool {
	return atomic.LoadUint32(&t.public) == 1
}

// Add adds a new claim to the merkle tree
// A claim is composed of two parts: index and value
//  1.index is mandatory, the data will be used for indexing the claim into to merkle tree
//  2.value is optional, the data will not affect the indexing
func (t *Tree) Add(index, value []byte) error {
	t.updateAccessTime()
	if len(index) < 4 {
		return fmt.Errorf("claim index too small (%d), minimum size is 4 bytes", len(index))
	}
	if len(index) > MaxKeySize || len(value) > MaxValueSize {
		return fmt.Errorf("index or value claim data too big")
	}
	if err := t.Tree.Add(index, value); err != nil {
		return err
	}
	_, err := t.store.Commit()
	return err
}

// GenProof generates a merkle tree proof that can be later used on CheckProof() to validate it
func (t *Tree) GenProof(index, value []byte) ([]byte, error) {
	t.updateAccessTime()
	proof, err := t.Tree.Proof(index)
	if err != nil {
		return nil, err
	}
	return proof, nil
}

// CheckProof standalone function for checking a merkle proof
func CheckProof(index, value, root []byte, mproof []byte) (bool, error) {
	if len(index) > gravitonstate.GravitonMaxKeySize {
		return false, fmt.Errorf("index is too big, maximum allow is %d", gravitonstate.GravitonMaxKeySize)
	}
	if len(value) > gravitonstate.GravitonMaxValueSize {
		return false, fmt.Errorf("value is too big, maximum allow is %d", gravitonstate.GravitonMaxValueSize)
	}
	if len(root) != gravitonstate.GravitonHashSizeBytes {
		return false, fmt.Errorf("root hash length is incorrect (expected %d)", gravitonstate.GravitonHashSizeBytes)
	}
	return gravitonstate.Verify(index, mproof, root)
}

// CheckProof validates a merkle proof and its data
func (t *Tree) CheckProof(index, value, root, mproof []byte) (bool, error) {
	if len(index) > gravitonstate.GravitonMaxKeySize {
		return false, fmt.Errorf("index is too big, maximum allow is %d", gravitonstate.GravitonMaxKeySize)
	}
	if len(value) > gravitonstate.GravitonMaxValueSize {
		return false, fmt.Errorf("value is too big, maximum allow is %d", gravitonstate.GravitonMaxValueSize)
	}
	if t.Tree == nil {
		return false, fmt.Errorf("tree %s does not exist", t.name)
	}
	t.updateAccessTime()
	return t.Tree.Verify(index, mproof, root), nil
}

// Root returns the current root hash of the merkle tree
func (t *Tree) Root() []byte {
	t.updateAccessTime()
	return t.Tree.Hash()
}

func (t *Tree) treeWithRoot(root []byte) statedb.StateTree {
	if root == nil {
		return t.Tree
	}
	return t.store.TreeWithRoot(root)
}

// Dump returns the whole merkle tree serialized in a format that can be used on Import.
// Byte seralization is performed using bare message protocol, it is a 40% size win over JSON
func (t *Tree) Dump(root []byte) ([]byte, error) {
	t.updateAccessTime()
	tree := t.treeWithRoot(root)
	if tree == nil {
		return nil, fmt.Errorf("dump: root not found %x", root)
	}
	dump := exportData{}
	tree.Iterate(nil, func(k, v []byte) bool {
		ee := exportElement{Key: make([]byte, len(k)), Value: make([]byte, len(v))}
		// Copy elements since it's not safe to hold on to the []byte values from Iterate
		copy(ee.Key, k[:])
		copy(ee.Value, v[:])
		dump.Elements = append(dump.Elements, ee)
		return false
	})
	return bare.Marshal(&dump)
}

// Size returns the number of leaf nodes on the merkle tree
func (t *Tree) Size(root []byte) (int64, error) {
	tree := t.treeWithRoot(root)
	if tree == nil {
		return 0, nil
	}
	return int64(tree.Count()), nil
}

// DumpPlain returns the entire list of added claims for a specific root hash
// First return parametre are the indexes and second the values
// If root is not specified, the current one is used
// If responseBase64 is true, the list will be returned base64 encoded
func (t *Tree) DumpPlain(root []byte) ([][]byte, [][]byte, error) {
	var indexes, values [][]byte
	var err error
	t.updateAccessTime()

	tree := t.treeWithRoot(root)
	if tree == nil {
		return nil, nil, fmt.Errorf("DumpPlain: root not found %x", root)
	}
	tree.Iterate(nil, func(k, v []byte) bool {
		indexes = append(indexes, k)
		values = append(values, v)
		return false
	})

	return indexes, values, err
}

// ImportDump imports a partial or whole tree previously exported with Dump()
func (t *Tree) ImportDump(data []byte) error {
	t.updateAccessTime()
	census := new(exportData)
	if err := bare.Unmarshal(data, census); err != nil {
		return fmt.Errorf("importdump cannot unmarshal data: %w", err)
	}
	for _, ee := range census.Elements {
		if err := t.Tree.Add(ee.Key, ee.Value); err != nil {
			return err
		}
	}
	_, err := t.store.Commit()
	return err
}

// Snapshot returns a Tree instance of a exiting merkle root
func (t *Tree) Snapshot(root []byte) (censustree.Tree, error) {
	tree := t.treeWithRoot(root)
	if tree == nil {
		return nil, fmt.Errorf("snapshot: root not valid or not found %s", root)
	}
	return &Tree{store: t.store, Tree: tree, public: t.public}, nil
}

// HashExists checks if a hash exists as a node in the merkle tree
func (t *Tree) HashExists(hash []byte) (bool, error) {
	t.updateAccessTime()
	tree := t.treeWithRoot(hash)
	if tree == nil {
		return false, nil
	}
	return true, nil
}
