package viewer

import "math"

// mat4 is a column-major 4x4 matrix, matching GL's uniform layout.
type mat4 [16]float32

// perspective builds a right-handed perspective projection (fovY in
// radians).
func perspective(fovY, aspect, near, far float32) mat4 {
	f := float32(1 / math.Tan(float64(fovY)/2))
	var m mat4
	m[0] = f / aspect
	m[5] = f
	m[10] = (far + near) / (near - far)
	m[11] = -1
	m[14] = (2 * far * near) / (near - far)
	return m
}

// lookAt builds a right-handed view matrix for a camera at eye looking at
// target, with up as the world up vector.
func lookAt(eye, target, up [3]float32) mat4 {
	f := normalize(sub(target, eye))
	s := normalize(cross(f, up))
	u := cross(s, f)

	var m mat4
	m[0], m[4], m[8] = s[0], s[1], s[2]
	m[1], m[5], m[9] = u[0], u[1], u[2]
	m[2], m[6], m[10] = -f[0], -f[1], -f[2]
	m[12] = -dot(s, eye)
	m[13] = -dot(u, eye)
	m[14] = dot(f, eye)
	m[15] = 1
	return m
}

// mul returns a*b (a applied after b, as in GL conventions).
func (a mat4) mul(b mat4) mat4 {
	var r mat4
	for col := 0; col < 4; col++ {
		for row := 0; row < 4; row++ {
			var sum float32
			for k := 0; k < 4; k++ {
				sum += a[k*4+row] * b[col*4+k]
			}
			r[col*4+row] = sum
		}
	}
	return r
}

func sub(a, b [3]float32) [3]float32 {
	return [3]float32{a[0] - b[0], a[1] - b[1], a[2] - b[2]}
}

func cross(a, b [3]float32) [3]float32 {
	return [3]float32{
		a[1]*b[2] - a[2]*b[1],
		a[2]*b[0] - a[0]*b[2],
		a[0]*b[1] - a[1]*b[0],
	}
}

func dot(a, b [3]float32) float32 {
	return a[0]*b[0] + a[1]*b[1] + a[2]*b[2]
}

func normalize(v [3]float32) [3]float32 {
	l := float32(math.Sqrt(float64(dot(v, v))))
	if l == 0 {
		return v
	}
	return [3]float32{v[0] / l, v[1] / l, v[2] / l}
}

// orbitCamera is a Z-up camera orbiting target at distance, controlled by
// yaw (around Z) and pitch (around the horizontal axis).
type orbitCamera struct {
	target           [3]float32
	yaw, pitch       float32
	distance, radius float32
}

// frame centres the camera on the bounds [min, max] and backs it off to fit
// the whole bounding sphere in a 45° vertical FOV.
func (c *orbitCamera) frame(min, max [3]float32) {
	c.target = [3]float32{
		(min[0] + max[0]) / 2,
		(min[1] + max[1]) / 2,
		(min[2] + max[2]) / 2,
	}
	c.radius = float32(math.Sqrt(float64(dot(sub(max, c.target), sub(max, c.target)))))
	if c.radius == 0 {
		c.radius = 1
	}
	c.yaw = 0
	c.pitch = 20 * math.Pi / 180
	c.distance = c.radius / float32(math.Sin(float64(fovY)/2)) * 1.1
}

// rotate adjusts yaw/pitch by a drag of dx, dy pixels.
func (c *orbitCamera) rotate(dx, dy float32) {
	const degPerPixel = 0.4 * math.Pi / 180
	c.yaw += dx * degPerPixel
	c.pitch += dy * degPerPixel

	const maxPitch = 89 * math.Pi / 180
	if c.pitch > maxPitch {
		c.pitch = maxPitch
	}
	if c.pitch < -maxPitch {
		c.pitch = -maxPitch
	}
}

// zoom scales the distance by 0.9^steps, clamped to [0.05, 50] x radius.
func (c *orbitCamera) zoom(steps float32) {
	c.distance *= float32(math.Pow(0.9, float64(steps)))

	min := 0.05 * c.radius
	max := 50 * c.radius
	if c.distance < min {
		c.distance = min
	}
	if c.distance > max {
		c.distance = max
	}
}

// fovY is the camera's vertical field of view, in radians.
const fovY = 45 * math.Pi / 180

// matrices returns the view and projection matrices for the current camera
// state and viewport aspect ratio.
func (c *orbitCamera) matrices(aspect float32) (view, proj mat4) {
	cosPitch := float32(math.Cos(float64(c.pitch)))
	eye := [3]float32{
		c.target[0] + c.distance*cosPitch*float32(math.Cos(float64(c.yaw))),
		c.target[1] + c.distance*cosPitch*float32(math.Sin(float64(c.yaw))),
		c.target[2] + c.distance*float32(math.Sin(float64(c.pitch))),
	}

	view = lookAt(eye, c.target, [3]float32{0, 0, 1})

	near := c.distance / 100
	far := c.distance + 2*c.radius
	proj = perspective(fovY, aspect, near, far)
	return view, proj
}
