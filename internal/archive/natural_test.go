package archive

import "testing"

func TestNaturalCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"page1.jpg", "page2.jpg", -1},
		{"page2.jpg", "page10.jpg", -1},
		{"page10.jpg", "page2.jpg", 1},
		{"page1.jpg", "page1.jpg", 0},
		{"page01.jpg", "page1.jpg", 1},
		{"a.jpg", "B.jpg", -1},
		{"A.jpg", "a.jpg", -1},
		{"sub/page1.jpg", "sub/page2.jpg", -1},
		{"sub\\page1.jpg", "sub/page2.jpg", -1},
		{"img2/page1.jpg", "img10/page1.jpg", -1},
		{"1.jpg", "01.jpg", -1},
		{"x", "x/y", -1},
	}
	for _, c := range cases {
		got := NaturalCompare(c.a, c.b)
		if sign(got) != c.want {
			t.Errorf("NaturalCompare(%q, %q) = %d, want sign %d", c.a, c.b, got, c.want)
		}
	}
}

func TestNaturalCompareIsTotal(t *testing.T) {
	// Distinct strings must never compare equal, otherwise sorting is unstable.
	pairs := [][2]string{
		{"a.jpg", "A.jpg"},
		{"page1.jpg", "page01.jpg"},
		{"x/y", "x\\y"},
	}
	for _, p := range pairs {
		if NaturalCompare(p[0], p[1]) == 0 {
			t.Errorf("NaturalCompare(%q, %q) = 0 for distinct strings", p[0], p[1])
		}
	}
}

func TestNaturalCompareAntisymmetric(t *testing.T) {
	names := []string{"a1.jpg", "a10.jpg", "a2.jpg", "B.jpg", "b1.png", "sub/c.jpg", "sub2/c.jpg"}
	for _, x := range names {
		for _, y := range names {
			xy := sign(NaturalCompare(x, y))
			yx := sign(NaturalCompare(y, x))
			if xy != -yx {
				t.Errorf("asymmetry: %q vs %q -> %d / %d", x, y, xy, yx)
			}
		}
	}
}

func TestIsImageName(t *testing.T) {
	yes := []string{"a.jpg", "a.JPEG", "a.png", "a.gif", "a.webp", "dir/a.jfif", "a.jpe"}
	no := []string{"a.txt", "a.html", "a.svg", "a.bmp", "a.tiff", "a", "a.jpg.txt"}
	for _, n := range yes {
		if !IsImageName(n) {
			t.Errorf("IsImageName(%q) = false, want true", n)
		}
	}
	for _, n := range no {
		if IsImageName(n) {
			t.Errorf("IsImageName(%q) = true, want false", n)
		}
	}
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}
