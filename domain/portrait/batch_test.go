package portrait

import "testing"

func TestBatchAddAssignsNeverReusedIDs(t *testing.T) {
	b := NewBatch([]string{"team/alice.jpg", "other/alice.jpg"}, DefaultFramingSpec())
	if b.Items[0].ID == b.Items[1].ID {
		t.Fatalf("two items share ID %d", b.Items[0].ID)
	}
	if b.Items[0].Name != "alice" {
		t.Errorf("Name = %q, want %q", b.Items[0].Name, "alice")
	}

	last := b.Items[1].ID
	b.Remove(last)
	if added := b.Add("bob.png"); added.ID == last {
		t.Errorf("Add reused removed ID %d", last)
	}
}

func TestBatchFindReplaceRemove(t *testing.T) {
	b := NewBatch([]string{"a.jpg", "b.jpg", "c.jpg"}, DefaultFramingSpec())
	middle := b.Items[1]

	middle.Name = "Bob"
	if !b.Replace(middle) {
		t.Fatal("Replace() = false for an existing item")
	}
	if got, _ := b.Find(middle.ID); got.Name != "Bob" {
		t.Errorf("Find() after Replace: Name = %q, want %q", got.Name, "Bob")
	}

	if !b.Remove(middle.ID) {
		t.Fatal("Remove() = false for an existing item")
	}
	if _, ok := b.Find(middle.ID); ok {
		t.Error("Find() still returns a removed item")
	}
	if len(b.Items) != 2 || b.Items[0].Name != "a" || b.Items[1].Name != "c" {
		t.Errorf("order not preserved after Remove: %+v", b.Items)
	}
	if b.Remove(middle.ID) || b.Replace(middle) {
		t.Error("Remove()/Replace() succeed on a missing item")
	}
}

func TestBatchRemoveDoesNotAlterCopies(t *testing.T) {
	b := NewBatch([]string{"a.jpg", "b.jpg", "c.jpg"}, DefaultFramingSpec())
	snapshot := b.Items
	b.Remove(b.Items[0].ID)
	if snapshot[0].Name != "a" || snapshot[1].Name != "b" {
		t.Errorf("Remove() mutated a previously shared slice: %+v", snapshot)
	}
}

func TestBatchSetDefaultSpecOverridesCustomSpecs(t *testing.T) {
	b := NewBatch([]string{"a.jpg", "b.jpg"}, DefaultFramingSpec())
	custom := b.Items[0]
	custom.Spec = custom.Spec.WithFraming(0.1, 0.1, 0)
	b.Replace(custom)
	if !b.HasCustomSpec(b.Items[0]) || b.HasCustomSpec(b.Items[1]) {
		t.Fatal("HasCustomSpec() does not reflect the override")
	}

	newDefault := DefaultFramingSpec().WithFraming(0.4, 0.6, 0)
	b.SetDefaultSpec(newDefault)
	for _, it := range b.Items {
		if it.Spec != newDefault || b.HasCustomSpec(it) {
			t.Errorf("item %d: Spec = %+v, want the new default %+v", it.ID, it.Spec, newDefault)
		}
	}
}
