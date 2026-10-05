// VENDORED COPY — canonical source: Web/tools/check-brand-geometry.cjs
//
// The guard lives in the docs repository so it can police every Gamaj surface at
// once. Each repository that ships a Gamaj surface also carries its own copy so
// the check runs in that repository's CI, where the other repositories are not
// present. Change the canonical file, then re-vendor; keep them byte-identical
// below this banner.

// Fails if any Gamaj asset, template or source file still carries retired
// branding.
//
// The identity has exactly one mark: the four-module symbol. The rounded tile
// with a cut-out "G", and the per-product node hexagon, are retired and must
// not come back.
//
// Files are selected by content, not by extension, because the retired strings
// can appear anywhere a template is embedded: Go sources and string literals,
// SCSS, locale JSON, the render tool, and shell scripts. Binary assets (PNG,
// ICO, fonts) are skipped.
//
// The author line "Coded by AsliCode" is NOT retired branding. It is the
// requested closing line of every Gamaj page, so it is deliberately absent
// from the list below.
//
// This guard also polices the palette: dashboard source may not name Chakra's
// default grey, alpha or chromatic scales at all, because the Gamaj identity
// is monochrome apart from three status hues, all reached by role.
//
//   node tools/check-brand-geometry.cjs
//
// The guard runs against a workspace root, which defaults to the directory
// two levels above this file. Both the root and the set of palette-governed
// directories can be overridden so the same file can be vendored into an
// individual repository and run in that repository's CI, where the other
// repositories are not present:
//
//   GAMAJ_BRAND_ROOT=/path/to/repo \
//   GAMAJ_PALETTE_DIRS=dashboard/src \
//   node tools/check-brand-geometry.cjs

const fs = require("node:fs");
const path = require("node:path");

// The dashboard may only name colours through the Gamaj palette: the panel
// roles, the gamaj neutral ramp, or the three status hues. Chakra's own
// default scales are a different palette that happens to be present in the
// library, so nothing may reach for them by name.
//
// Built at runtime so this guard does not contain the tokens it forbids,
// which would otherwise make it flag its own source.
const OFF_PALETTE = buildOffPalette();

function buildOffPalette() {
	// Every numeric step of Chakra's neutral and alpha scales.
	const ramps = [
		"gray",
		"white" + "Alpha",
		"black" + "Alpha",
		// The default accent scale, which is the neutral ramp wearing another
		// name; the Gamaj theme remaps it, but the name should not survive.
		"primary",
		// The chromatic scales. The identity allows exactly three hues and
		// they are reached through panel.danger / warning / success.
		"red",
		"orange",
		"yellow",
		"green",
		"teal",
		"cyan",
		"blue",
		"purple",
		"pink",
	];
	const steps = [];
	for (let i = 50; i <= 950; i += 50) steps.push(String(i));
	// Chakra also offers 100, 200, 300 and 400 in its alpha scales.
	steps.push("100", "200", "300", "400");
	const out = [];
	for (const ramp of ramps) {
		for (const step of steps) out.push(`${ramp}.${step}`);
	}
	return out;
}

/**
 * The token is only a violation when it is used as a value. Matching the bare
 * name would also hit prose, a CSS class name, or the guard's own table, so
 * the match requires a quote or a backtick on at least one side.
 */
// Chakra spells a scale step `gray.400` in a token but `gray-400` in the CSS
// variable it compiles to, so the var() form needs the other separator.
const offPaletteAlt = OFF_PALETTE.map((t) => t.replace(".", "\\.")).join("|");
const offPaletteVarAlt = OFF_PALETTE.map((t) => t.replace(".", "-")).join("|");

const OFF_PALETTE_USE = new RegExp(
	// A quoted token: the usual `color="gray.500"`.
	`(["'\`])(?:${offPaletteAlt})\\1` +
		// The same scales reached through a var() rather than a quoted token.
		// These resolve on-palette because the theme remaps them, but naming
		// them still ties the source to Chakra's default scales, and the remap
		// is exactly the sort of thing that can be dropped in a later upgrade.
		`|var\\(--chakra-colors-(?:${offPaletteVarAlt})\\)`,
	"g",
);

const PALETTE_EXT = /\.(tsx|ts)$/;

/**
 * A token purge cannot see a colour that was written out by hand, so the
 * guard also rejects raw literals. These are the files allowed to carry them,
 * each for a stated reason:
 *
 *   index.scss          it is where the palette is defined
 *   JsonEditor/styles   Ace's own syntax theme, shipped verbatim
 *   AccountSecurity     operating-system and vendor brand marks
 *   OperatorIdentity    ISP logos, in their own brand colours
 *   SponsorCarousel     a sponsor's wordmark, in its own ink
 *   CoreSettingsPage    NordVPN / TorProject marks
 *   themeColor.ts       a <meta> content attribute, which takes no var()
 *   imperial-iran-flag  a national flag, which is not ours to recolour
 *   assets/operators/   ISP logos, each in its own brand ink
 *
 * Paths are relative to that surface's own root. A `:root { ... }` theme block
 * is exempt by line, because that is where a surface declares its ramp.
 */
const LITERAL_EXEMPT = new Set([
	"index.scss",
	"components/JsonEditor/styles.css",
	"components/AccountSecurity.tsx",
	"components/OperatorIdentity.tsx",
	"components/SponsorCarousel.tsx",
	"pages/CoreSettingsPage.tsx",
	"utils/themeColor.ts",
	"assets/imperial-iran-flag.svg",
]);

/**
 * The bot panel is one self-contained HTML file, so it has to carry the ramp
 * itself. Only the block that defines that ramp is exempt: exempting the file
 * would hide every colour written anywhere in it, which is exactly the bug
 * this guard exists to catch.
 *
 * Returns the line numbers of any `:root { ... }` / `html[data-theme] { ... }`
 * block, i.e. the theme's own declarations.
 */
/**
 * Lines inside a palette *definition* block are the one place a raw value is
 * allowed, because naming the ramp is what the rest of the file then points
 * at. The blocks recognised are the shared roots, and the standalone
 * surfaces that declare their own colour-mode blocks: the subscription page
 * keeps `.rb-theme-light` / `.rb-theme-dark` beside its `:root` because it is
 * a single HTML file served on its own, with no shared stylesheet to inherit
 * from.
 */
function paletteDefinitionLines(lines) {
	const exempt = new Set();
	let open = false;
	lines.forEach((line, i) => {
		if (
			!open &&
			/^\s*(:root|html\[data-theme[^\]]*\]|\.[a-z0-9_-]*theme-(light|dark))\s*\{/.test(
				line,
			)
		) {
			open = true;
		}
		if (open) {
			exempt.add(i);
			if (/\}\s*$/.test(line.trim()) && /^\s*\}/.test(line) === false) {
				/* the closing brace is on its own line; handled below */
			}
			if (/^\s*\}\s*$/.test(line)) open = false;
		}
	});
	return exempt;
}

/** Extensions whose colour can be written as a literal rather than a token. */
// A colour can be written out by hand in a translation string just as easily
// as in a stylesheet, and a hex in a locale file reaches the screen verbatim,
// so JSON is scanned alongside the source extensions.
const LITERAL_EXT = /\.(tsx|ts|css|scss|svg|html|json)$/;

/**
 * Hex in a valid CSS length — 3, 4, 6 or 8 digits, the last carrying an
 * alpha channel — or an rgb()/rgba() triple.
 *
 * The lengths are listed longest-first rather than written as a range: a
 * `{6}\b` misses an 8-digit colour with an alpha channel entirely, and a
 * `{3,8}` swallows the first 8 digits of any longer hex-looking run. Only the
 * four real CSS lengths match, so a commit hash in a comment is not a hit.
 */
const RAW_COLOUR =
	/#[0-9a-fA-F]{8}\b|#[0-9a-fA-F]{6}\b|#[0-9a-fA-F]{4}\b|#[0-9a-fA-F]{3}\b|\brgba?\(\s*\d+/g;

/**
 * A path prefix that exempts a whole directory, for a family of one-off
 * assets.
 *
 * `statics/favicon/` is here because a pinned-tab mask is rendered by the
 * browser as a standalone SVG document: it has no stylesheet, so a custom
 * property never resolves and the mark's fill has to be written out. The
 * literal is the palette's own black, not a colour invented for the icon.
 */
const LITERAL_EXEMPT_DIRS = ["assets/operators/", "statics/favicon/"];

const ROOT = path.resolve(
	process.env.GAMAJ_BRAND_ROOT || path.join(__dirname, "..", ".."),
);

// Assembled at runtime so this file never contains the literals itself,
// which would otherwise make the guard flag its own source.
const RETIRED = retiredMarks();

function retiredMarks() {
	const cut = "gamaj" + "-cut";
	const nodeCut = "gamaj-node" + "-cut";
	return [
		"squared-off " + "G",
		cut,
		nodeCut,
		"cut out of a solid app " + "tile",
		"the Gamaj " + "G in a hexagon",
	];
}

/** Generated output and dependencies that are not committed. */
const SKIP_DIRS = new Set([
	"node_modules",
	".git",
	"build",
	"dist",
	"vendor",
	"static",
]);

/** Files larger than this are almost certainly binary assets. */
const MAX_BYTES = 4 * 1024 * 1024;

let scanned = 0;
let skippedBinary = 0;
const offenders = [];

function looksBinary(buffer) {
	// A NUL byte in the first block is the standard heuristic.
	const limit = Math.min(buffer.length, 4096);
	for (let i = 0; i < limit; i++) {
		if (buffer[i] === 0) return true;
	}
	return false;
}

function walk(dir) {
	for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
		if (SKIP_DIRS.has(entry.name)) continue;
		const p = path.join(dir, entry.name);
		if (entry.isDirectory()) {
			walk(p);
			continue;
		}
		if (!entry.isFile()) continue;

		let buffer;
		try {
			const stat = fs.statSync(p);
			if (stat.size > MAX_BYTES) {
				skippedBinary++;
				continue;
			}
			buffer = fs.readFileSync(p);
		} catch {
			continue;
		}
		if (looksBinary(buffer)) {
			skippedBinary++;
			continue;
		}

		scanned++;
		const text = buffer.toString("utf8");
		for (const needle of RETIRED) {
			if (text.includes(needle)) {
				offenders.push(`${path.relative(ROOT, p)} -> "${needle}"`);
				break;
			}
		}
	}
}

walk(ROOT);

/* ---------------------------------------------------------------- palette */

const paletteOffenders = [];
let paletteScanned = 0;

/**
 * Every Gamaj surface is governed by the palette rule, not just the
 * dashboard: the bot panel is a standalone HTML file and the node ships no
 * UI of its own. Paths are relative to the workspace root.
 */
/**
 * Every Gamaj surface is governed by the palette rule, not just the
 * dashboard: the bot panel is a standalone HTML file, and the default
 * subscription page is a third one that users reach at their own link.
 *
 * An entry may be a directory or a single file. The subscription directory
 * holds two alternate templates that are opt-in and not yet on the palette,
 * so only the default one is named here rather than opting the whole folder
 * in. Paths are relative to the workspace root.
 */
const PALETTE_DIRS = (
	process.env.GAMAJ_PALETTE_DIRS ??
	"Panel/dashboard/src,Panel/dashboard/public,Panel/templates/subscription/index.html,Bot/internal/botpanel"
)
	.split(",")
	.map((entry) => entry.trim())
	.filter(Boolean)
	.map((entry) => path.join(ROOT, entry));

/**
 * Check one file against the palette rule. `rel` is the path used for the
 * exemption lists, which are written relative to a governed root.
 */
function checkPaletteFile(p, rel) {
	if (!LITERAL_EXT.test(p)) return;
	const text = fs.readFileSync(p, "utf8");
	paletteScanned++;
	const lines = text.split("\n");
	const definitionLines = paletteDefinitionLines(lines);
	lines.forEach((line, i) => {
		OFF_PALETTE_USE.lastIndex = 0;
		let match;
		const seen = new Set();
		while ((match = OFF_PALETTE_USE.exec(line)) !== null) {
			if (seen.has(match[0])) continue;
			seen.add(match[0]);
			paletteOffenders.push(
				`${path.relative(ROOT, p)}:${i + 1} -> ${match[0]}`,
			);
		}
		// A token purge cannot see a colour written out by hand, so a
		// raw literal is policed here too.
		if (
			LITERAL_EXEMPT.has(rel) ||
			definitionLines.has(i) ||
			LITERAL_EXEMPT_DIRS.some((dir) => rel.startsWith(dir))
		) {
			return;
		}
		RAW_COLOUR.lastIndex = 0;
		const raw = new Set();
		while ((match = RAW_COLOUR.exec(line)) !== null) {
			raw.add(match[0]);
		}
		for (const value of raw) {
			paletteOffenders.push(
				`${path.relative(ROOT, p)}:${i + 1} -> raw literal ${value}`,
			);
		}
	});
}

for (const PALETTE_DIR of PALETTE_DIRS) {
if (!fs.existsSync(PALETTE_DIR)) continue;
{
	// A single file entry is checked directly; a directory is walked.
	if (fs.statSync(PALETTE_DIR).isFile()) {
		checkPaletteFile(PALETTE_DIR, path.basename(PALETTE_DIR));
		continue;
	}
	const walkPalette = (dir) => {
		for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
			const p = path.join(dir, entry.name);
			if (entry.isDirectory()) {
				if (entry.name === "node_modules") continue;
				walkPalette(p);
				continue;
			}
			if (!entry.isFile()) continue;
			const rel = path
				.relative(PALETTE_DIR, p)
				.replace(/\\/g, "/");
			checkPaletteFile(p, rel);
		}
	};
	walkPalette(PALETTE_DIR);
}
}

if (paletteOffenders.length > 0) {
	console.error("check-brand-geometry: FAILED (off-palette colour tokens)");
	for (const o of paletteOffenders) console.error(`  ${o}`);
	console.error(
		"Gamaj surfaces name colours only through the palette:\n" +
			"  panel.text / textSecondary / textMuted   panel.accent / accentHover\n" +
			"  panel.app / main / surface / elevated / inset\n" +
			"  panel.border / borderStrong / rowHover / rowSelected / scrim\n" +
			"  panel.danger / dangerSubtle / panel.warning / panel.success\n" +
			"  gamaj.<step> for the neutral ramp, var(--gm-*) for a raw value.\n" +
			"Chakra's default gray, alpha and chromatic scales are a different\n" +
			"palette and must not be named, and a raw hex or rgb() literal must\n" +
			"not be written out either. A few files are exempt by name; see\n" +
			"LITERAL_EXEMPT in this guard for the reasons.",
	);
	process.exit(1);
}

if (offenders.length > 0) {
	console.error("check-brand-geometry: FAILED");
	for (const o of offenders) console.error(`  ${o}`);
	console.error(
		"Retired branding must not reappear. Every Gamaj surface uses the\n" +
			"four-module symbol, never a second logo.",
	);
	process.exit(1);
}

console.log(
	`check-brand-geometry: OK (${scanned} text files scanned, ` +
		`${skippedBinary} binary/large skipped, no retired branding; ` +
		`${paletteScanned} Gamaj surface sources on the palette)`,
);
