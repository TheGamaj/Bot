// Renders the Gamaj brand mark to PNG/ICO favicon assets.
//
// The mark is defined once as vector geometry in the SVG files under
// assets/img/. Favicon raster sizes are generated from the same geometry here
// so the tab icon can never drift from the logo:
//
//   node tools/render-brand-assets.mjs <output-dir>
//
// Geometry (512x512 viewBox, matching assets/img/gamaj-logo.svg):
//   - a rounded tile
//   - a geometric G cut out of the tile: an annulus with a gap in the
//     upper-right quadrant plus the horizontal bar
//
// The raster uses an opaque dark tile with a white G so the icon stays legible
// on both light and dark browser chrome.

import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { deflateSync } from "node:zlib";

const SIZE = 512;
const TILE = { x: 24, y: 24, w: 464, h: 464, r: 128 };
const RING = { cx: 256, cy: 256, outer: 142, inner: 82 };
// The G's opening: the ring is cut between these polar angles (degrees, math
// convention with y pointing up).
const GAP = { from: -3, to: 53.9 };
const BAR = { x0: 288, x1: 368, y0: 220, y1: 280 };

const TILE_COLOR = [0x0b, 0x0b, 0x0f];
const MARK_COLOR = [0xff, 0xff, 0xff];

function insideRoundedRect(x, y, { x: rx, y: ry, w, h, r }) {
	const cx = Math.min(Math.max(x, rx + r), rx + w - r);
	const cy = Math.min(Math.max(y, ry + r), ry + h - r);
	const dx = x - cx;
	const dy = y - cy;
	return dx * dx + dy * dy <= r * r;
}

function insideMark(x, y) {
	const dx = x - RING.cx;
	const dy = RING.cy - y; // flip to math convention
	const radius = Math.hypot(dx, dy);
	if (radius >= RING.inner && radius <= RING.outer) {
		const angle = (Math.atan2(dy, dx) * 180) / Math.PI;
		if (angle < GAP.from || angle > GAP.to) return true;
	}
	return x >= BAR.x0 && x <= BAR.x1 && y >= BAR.y0 && y <= BAR.y1;
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
					if (!insideRoundedRect(x, y, TILE)) continue;
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
				pixels[offset + channel] = Math.round(tile + (mark - tile) * markShare);
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