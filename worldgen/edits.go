package worldgen

import "encoding/binary"

// editOverlay is the published, append-only cell override layer.
// Published overlays are never mutated in place: apply returns a new
// overlay, so an in-flight generation always reads a stable snapshot.
type editOverlay struct {
	m map[[2]int]CellEdit
}

func newEditOverlay() *editOverlay {
	return &editOverlay{m: map[[2]int]CellEdit{}}
}

func (o *editOverlay) get(x, y int) (CellEdit, bool) {
	if o == nil {
		return CellEdit{}, false
	}
	e, ok := o.m[[2]int{x, y}]
	return e, ok
}

// apply returns a NEW overlay containing the receiver's edits plus the
// batch. Merge semantics make the published state order-independent:
//
//   - within a batch, entries are folded left to right over the
//     existing value, so later entries in the submitted slice win per
//     field;
//   - nil fields leave the previous value untouched (patch semantics);
//   - submitting the same batch twice is idempotent, and reordering
//     batches that touch disjoint cells cannot change the result.
func (o *editOverlay) apply(edits []CellEdit) *editOverlay {
	next := &editOverlay{m: make(map[[2]int]CellEdit, len(o.m)+len(edits))}
	for k, v := range o.m {
		next.m[k] = v
	}
	for _, e := range edits {
		k := [2]int{e.X, e.Y}
		cur := next.m[k]
		cur.X, cur.Y = e.X, e.Y
		if e.SetHeight != nil {
			v := *e.SetHeight
			cur.SetHeight = &v
		}
		if e.SetMoisture != nil {
			v := *e.SetMoisture
			cur.SetMoisture = &v
		}
		next.m[k] = cur
	}
	return next
}

// len reports the number of edited cells.
func (o *editOverlay) len() int {
	if o == nil {
		return 0
	}
	return len(o.m)
}

// encode produces canonical big-endian serialization, sorted by
// (x,y). Two worlds with the same logical edits always encode to the
// same bytes.
func (o *editOverlay) encode() []byte {
	keys := make([][2]int, 0, len(o.m))
	for k := range o.m {
		keys = append(keys, k)
	}
	// sort by y then x for canonical order
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0; j-- {
			a, b := keys[j-1], keys[j]
			if a[1] < b[1] || (a[1] == b[1] && a[0] < b[0]) {
				break
			}
			keys[j-1], keys[j] = b, a
		}
	}
	buf := make([]byte, 0, 16+len(keys)*14)
	buf = binary.BigEndian.AppendUint32(buf, uint32(len(keys)))
	for _, k := range keys {
		e := o.m[k]
		buf = binary.BigEndian.AppendUint32(buf, uint32(int64(k[0])))
		buf = binary.BigEndian.AppendUint32(buf, uint32(int64(k[1])))
		var flags uint16
		if e.SetHeight != nil {
			flags |= 1
		}
		if e.SetMoisture != nil {
			flags |= 2
		}
		buf = binary.BigEndian.AppendUint16(buf, flags)
		if e.SetHeight != nil {
			buf = binary.BigEndian.AppendUint16(buf, uint16(*e.SetHeight))
		}
		if e.SetMoisture != nil {
			buf = binary.BigEndian.AppendUint16(buf, uint16(*e.SetMoisture))
		}
	}
	return buf
}
