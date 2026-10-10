// JSDoc typedefs for the JSON and binary shapes served by web.Server (see
// the modelView/web package doc comment, the source of truth for this
// API), plus fetch helpers for them. Binary decoders live in mesh.js.

/** The most node ids/weights any vertex carries (mdx.MaxInfluences). */
export const MAX_INFLUENCES = 8;

/**
 * @typedef {Object} TextureRef
 * @property {string} path
 * @property {string} ckey - 32 lower-case hex characters
 * @property {string} url - GET this for the decoded .dds bytes
 */

/**
 * @typedef {Object} TeamTextures
 * @property {TextureRef} color
 * @property {TextureRef} glow
 */

/**
 * @typedef {Object} ModelListEntry
 * @property {string} path
 * @property {string} name
 * @property {string} edition - "classic" | "reforged" | "definitive" | "install"
 * @property {string[]} overlays
 */

/**
 * @typedef {Object} ModelList
 * @property {ModelListEntry[]} models - sorted by path
 * @property {TeamTextures[]} team - team[i] is player i, 0-24
 */

/**
 * @typedef {Object} Edition
 * @property {string} path
 * @property {string} edition
 * @property {string[]} overlays
 */

/**
 * @typedef {Object} Sequence
 * @property {string} name
 * @property {string} key - mdx.SequenceKey(name), for matching across editions
 * @property {number} start
 * @property {number} end
 */

/**
 * MeshOffsets are byte offsets into the buffer served by GET /api/mesh;
 * lengths follow from the owning Geoset's vertices/triangles.
 * @typedef {Object} MeshOffsets
 * @property {number} positions
 * @property {number} normals
 * @property {number[]} uvs - one offset per UV set
 * @property {number} weights
 * @property {number} nodes
 * @property {number} indices
 */

/**
 * @typedef {Object} Layer
 * @property {number} filter - mdx.FilterMode
 * @property {number} shading
 * @property {number} alpha
 * @property {number} uvSet
 * @property {boolean} wrapWidth
 * @property {boolean} wrapHeight
 * @property {string} source - "texture" | "teamColor" | "teamGlow" | "grey"
 * @property {string} label
 * @property {?TextureRef} texture
 * @property {?TextureRef} mask - the ORM; apply only when its DDS is DXT5
 */

/**
 * @typedef {Object} Geoset
 * @property {number} material
 * @property {number} lod
 * @property {number} vertices
 * @property {number} triangles
 * @property {number} priority
 * @property {number} alpha - at the default sequence's start, global clock 0
 * @property {Layer[]} layers
 * @property {?MeshOffsets} mesh - null for a geoset not at the model's lod
 */

/**
 * @typedef {Object} MeshRef
 * @property {string} url
 * @property {number} size - bytes
 */

/**
 * @typedef {Object} Model
 * @property {string} path
 * @property {string} name
 * @property {string} edition
 * @property {string[]} overlays
 * @property {string} ckey
 * @property {number} version
 * @property {number} chunks
 * @property {number} objects - len(Pivots()), the pose's matrix count
 * @property {number} lod - the lowest LOD, the one in the mesh
 * @property {number} vertices - total over every geoset
 * @property {number} triangles - total over every geoset
 * @property {Geoset[]} geosets
 * @property {Sequence[]} sequences
 * @property {number} defaultSequence - index into sequences, -1 when none
 * @property {number[]} globalSequences
 * @property {Edition[]} editions - every edition of this asset, including this one
 * @property {MeshRef} mesh
 */

/**
 * The binary layout of GET /api/mesh: per lowest-LOD geoset, in geoset
 * order: positions float32[3v], normals float32[3v] (zeros when the
 * geoset has none), each UV set float32[2v], weights float32[8v], node
 * object ids uint32[8v] (MAX_INFLUENCES per vertex), indices uint16[3t]
 * padded to 4 bytes. A Geoset's "mesh" MeshOffsets give byte offsets into
 * this buffer.
 */

/**
 * The binary layout of GET /api/pose?path=&seq=&frame=&global=: uint32
 * object count, uint32 geoset count, then object count x 16 float32 world
 * matrices (column-major, indexed by object id), then geoset count x
 * float32 alphas. seq is an index into Model.sequences or -1 (bind pose);
 * frame and global are uint32; all three are required.
 */

/**
 * checkOK throws with the status and body text when resp isn't a 2xx.
 * @param {Response} resp
 * @returns {Promise<Response>}
 */
async function checkOK(resp) {
	if (!resp.ok) {
		const body = await resp.text();
		throw new Error(`${resp.status} ${resp.statusText}: ${body}`);
	}
	return resp;
}

/**
 * getModels fetches GET /api/models.
 * @returns {Promise<ModelList>}
 */
export async function getModels() {
	const resp = await checkOK(await fetch("/api/models"));
	return resp.json();
}

/**
 * getModel fetches GET /api/model?path=.
 * @param {string} path
 * @returns {Promise<Model>}
 */
export async function getModel(path) {
	const resp = await checkOK(await fetch("/api/model?path=" + encodeURIComponent(path)));
	return resp.json();
}

/**
 * getMesh fetches model.mesh.url (GET /api/mesh?path=).
 * @param {Model} model
 * @returns {Promise<ArrayBuffer>}
 */
export async function getMesh(model) {
	const resp = await checkOK(await fetch(model.mesh.url));
	return resp.arrayBuffer();
}

/**
 * getTexture fetches ref.url (GET /api/texture?ckey=), the raw .dds bytes.
 * @param {TextureRef} ref
 * @returns {Promise<ArrayBuffer>}
 */
export async function getTexture(ref) {
	const resp = await checkOK(await fetch(ref.url));
	return resp.arrayBuffer();
}
