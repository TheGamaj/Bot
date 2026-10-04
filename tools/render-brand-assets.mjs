// Renders the Gamaj brand mark to PNG/ICO favicon assets.
//
// The mark is defined once as vector geometry, in the 64x64 unit grid taken
// from the Gamaj brand identity reference, and the favicon raster sizes are
// generated from that same geometry here so the tab icon can never drift from
// the logo:
//
//   node tools/render-brand-assets.mjs <output-dir>
//
// Geometry (64x64 unit grid, matching assets/img/gamaj-mark.svg):
//   - three equal squares in the top-left, top-right and bottom-left cells
//   - one smaller square in the bottom-right cell, creating the deliberate
//     asymmetry the identity calls for
//
// The raster uses an opaque black tile with a white mark so the icon stays
// legible on both light and dark browser chrome.

import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { deflateSync } from "node:zlib";

const SIZE = 512;

// The mark in its native 64x64 unit grid. Three 20x20 modules on a 28 unit
// pitch (20 module + 8 gap, offset 8) plus one 12x12 module bottom-right.
const UNIT_MARK = [
	{ x: 8, y: 8, size: 20 },
	{ x: 36, y: 8, size: 20 },
	{ x: 8, y: 36, size: 20 },
	{ x: 36, y: 44, size: 12 },
];
// The mark's bounding box in that grid: 8..56 on both axes, so 48x48 units.
const MARK_BOX = { x: 8, y: 8, span: 48 };

// The app-icon container: a black rounded square holding the white mark, per
// the brand identity (22% corner radius, mark inset to two thirds of the tile).
const TILE_RADIUS_RATIO = 0.22;
const MARK_TILE_RATIO = 2 / 3;

const TILE_COLOR = [0x00, 0x00, 0x00];
const MARK_COLOR = [0xff, 0xff, 0xff];

// Converts the 64 unit mark into 512 pixel modules centred in the tile.
function markModules() {
	const scale = (SIZE * MARK_TILE_RATIO) / MARK_BOX.span;
	const offset = (SIZE - MARK_BOX.span * scale) / 2 - MARK_BOX.x * scale;
	return UNIT_MARK.map(({ x, y, size }) => ({
		x: x * scale + offset,
		y: y * scale + offset,
		size: size * scale,
	}));
}

const MODULES = markModules();
const TILE_RADIUS = SIZE * TILE_RADIUS_RATIO;

function insideRoundedRect(x, y, { rx, ry, w, h, r }) {
	const cx = Math.min(Math.max(x, rx + r), rx + w - r);
	const cy = Math.min(Math.max(y, ry + r), ry + h - r);
	const dx = x - cx;
	const dy = y - cy;
	return dx * dx + dy * dy <= r * r;
}

function insideMark(x, y) {
	return MODULES.some(
		(m) => x >= m.x && x <= m.x + m.size && y >= m.y && y <= m.y + m.size,
	);
}

// Renders the mark at `size` with 4x4 supersampling and returns RGBA pixels.
function render(size) {
	const scale = SIZE / size;
	const samples = 4;
	const pixels = Buffer.alloc(size * size * 4);
	for (let py = 0; py < size; py++) {
		for (let px = 0; px < size; px++) {
			let tileHits = 0;
			let markHits = 0;
			for (let sy = 0; sy < samples; sy++) {
				for (let sx = 0; sx < samples; sx++) {
					const x = (px + (sx + 0.5) / samples) * scale;
					const y = (py + (sy + 0.5) / samples) * scale;
					if (!insideRoundedRect(x, y, {
						rx: 0,
						ry: 0,
						w: SIZE,
						h: SIZE,
						r: TILE_RADIUS,
					}))
						continue;
					tileHits++;
					if (insideMark(x, y)) markHits++;
				}
			}
			const total = samples * samples;
			const alpha = Math.round((tileHits / total) * 255);
			const markShare = tileHits === 0 ? 0 : markHits / tileHits;
			const offset = (py * size + px) * 4;
			for (let channel = 0; channel < 3; channel++) {
				const tile = TILE_COLOR[channel];
				const mark = MARK_COLOR[channel];
				pixels[offset + channel] = Math.round(
					tile + (mark - tile) * markShare,
				);
			}
			pixels[offset + 3] = alpha;
		}
	}
	return pixels;
}

const CRC_TABLE = (() => {
	const table = new Int32Array(256);
	for (let n = 0; n < 256; n++) {
		let c = n;
		for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
		table[n] = c;
	}
	return table;
})();

function crc32(buffer) {
	let c = -1;
	for (const byte of buffer) c = CRC_TABLE[(c ^ byte) & 0xff] ^ (c >>> 8);
	return (c ^ -1) >>> 0;
}

function chunk(type, data) {
	const length = Buffer.alloc(4);
	length.writeUInt32BE(data.length);
	const body = Buffer.concat([Buffer.from(type, "ascii"), data]);
	const crc = Buffer.alloc(4);
	crc.writeUInt32BE(crc32(body));
	return Buffer.concat([length, body, crc]);
}

function encodePng(size, pixels) {
	const ihdr = Buffer.alloc(13);
	ihdr.writeUInt32BE(size, 0);
	ihdr.writeUInt32BE(size, 4);
	ihdr[8] = 8; // bit depth
	ihdr[9] = 6; // RGBA
	const stride = size * 4;
	const raw = Buffer.alloc((stride + 1) * size);
	for (let y = 0; y < size; y++) {
		raw[y * (stride + 1)] = 0; // no filter
		pixels.copy(raw, y * (stride + 1) + 1, y * stride, (y + 1) * stride);
	}
	return Buffer.concat([
		Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
		chunk("IHDR", ihdr),
		chunk("IDAT", deflateSync(raw, { level: 9 })),
		chunk("IEND", Buffer.alloc(0)),
	]);
}

// ICO container with PNG payloads (supported by every current browser).
function encodeIco(entries) {
	const header = Buffer.alloc(6);
	header.writeUInt16LE(0, 0);
	header.writeUInt16LE(1, 2);
	header.writeUInt16LE(entries.length, 4);
	const directory = Buffer.alloc(16 * entries.length);
	let offset = header.length + directory.length;
	const bodies = [];
	entries.forEach((entry, index) => {
		const base = index * 16;
		directory[base] = entry.size >= 256 ? 0 : entry.size;
		directory[base + 1] = entry.size >= 256 ? 0 : entry.size;
		directory[base + 2] = 0;
		directory[base + 3] = 0;
		directory.writeUInt16LE(1, base + 4);
		directory.writeUInt16LE(32, base + 6);
		directory.writeUInt32LE(entry.png.length, base + 8);
		directory.writeUInt32LE(offset, base + 12);
		bodies.push(entry.png);
		offset += entry.png.length;
	});
	return Buffer.concat([header, directory, ...bodies]);
}

const outputDir = process.argv[2] ?? ".";
mkdirSync(outputDir, { recursive: true });

const outputs = [
	["favicon-16x16.png", 16],
	["favicon-32x32.png", 32],
	["favicon.png", 256],
	["apple-touch-icon.png", 180],
	["android-chrome-192x192.png", 192],
	["android-chrome-512x512.png", 512],
	["mstile-150x150.png", 150],
];

for (const [name, size] of outputs) {
	writeFileSync(join(outputDir, name), encodePng(size, render(size)));
	console.log(`wrote ${name} (${size}x${size})`);
}

const icoSizes = [16, 32, 48];
writeFileSync(
	join(outputDir, "favicon.ico"),
	encodeIco(icoSizes.map((size) => ({ size, png: encodePng(size, render(size)) }))),
);
console.log(`wrote favicon.ico (${icoSizes.join("/")})`);

// The mask icon Safari pins to the tab is a solid monochrome silhouette, so it
// is generated here too instead of being hand-maintained per repository.
const maskModules = MODULES.map(
	({ x, y, size: s }) =>
		`M${x.toFixed(2)} ${y.toFixed(2)}h${s.toFixed(2)}v${s.toFixed(2)}h-${s.toFixed(2)}z`,
).join("");
writeFileSync(
	join(outputDir, "safari-pinned-tab.svg"),
	`<svg version="1.1" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${SIZE} ${SIZE}">
  <!-- GAMAJ mark (monochrome): four modules on a modular grid. -->
  <path d="${maskModules}" fill="#000000"/>
</svg>
`,
);
console.log("wrote safari-pinned-tab.svg");
