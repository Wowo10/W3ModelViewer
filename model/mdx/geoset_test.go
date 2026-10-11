package mdx

import "testing"

func geosetWith(lod uint32, vertices, triangles int) Geoset {
	return Geoset{
		LOD:       lod,
		Positions: make([]float32, 3*vertices),
		Indices:   make([]uint16, 3*triangles),
	}
}

func TestLODTotals(t *testing.T) {
	if got := LODTotals(nil); got != nil {
		t.Errorf("LODTotals(nil) = %v, want nil", got)
	}

	one := []Geoset{geosetWith(0, 10, 5)}
	got := LODTotals(one)
	want := []LODTotal{{LOD: 0, Geosets: 1, Vertices: 10, Triangles: 5}}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("LODTotals(one LOD) = %v, want %v", got, want)
	}
	if got[0].LOD != LowestLOD(one) {
		t.Errorf("LODTotals(one)[0].LOD = %d, want LowestLOD = %d", got[0].LOD, LowestLOD(one))
	}

	interleaved := []Geoset{
		geosetWith(0, 10, 5),
		geosetWith(1, 8, 4),
		geosetWith(0, 12, 6),
		geosetWith(3, 3, 1),
		geosetWith(1, 7, 3),
	}
	got = LODTotals(interleaved)
	want = []LODTotal{
		{LOD: 0, Geosets: 2, Vertices: 22, Triangles: 11},
		{LOD: 1, Geosets: 2, Vertices: 15, Triangles: 7},
		{LOD: 3, Geosets: 1, Vertices: 3, Triangles: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("LODTotals(interleaved) = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("LODTotals(interleaved)[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	if got[0].LOD != LowestLOD(interleaved) {
		t.Errorf("LODTotals(interleaved)[0].LOD = %d, want LowestLOD = %d", got[0].LOD, LowestLOD(interleaved))
	}
}
