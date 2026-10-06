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
// The third rule polices logo geometry itself. The colour rule above could not
// catch the worst defect this workspace ever shipped: a hand-drawn mascot
// standing in for the symbol, on a surface that was otherwise perfectly
// on-palette. Nothing about it was off-colour, so the guard stayed green.
//
//   node tools/check-brand-geometry.cjs
//
// The guard runs against a workspace root, which defaults to the directory
// two levels above this file. The root and each set of governed directories can
// be overridden so the same file can be vendored into an individual repository
// and run in that repository's CI, where the other repositories are absent:
//
//   GAMAJ_BRAND_ROOT=/path/to/repo \
//   GAMAJ_PALETTE_DIRS=dashboard/src \
//   GAMAJ_LOGO_DIRS=templates/subscription \
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
 *   gamaj-mark.svg      a standalone SVG document: no stylesheet, so no var()
 *   gamaj-logo.svg      idem, for the lockup
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
	// The mark and the lockup, as shipped assets. They are read as standalone
	// SVG documents - by a README, a tab strip, a pinned tab - which have no
	// stylesheet and so cannot resolve a custom property. The two values are
	// the palette's own white and black, chosen by colour scheme, not colours
	// invented for the logo.
	"assets/gamaj-mark.svg",
	"assets/gamaj-logo.svg",
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

/* ------------------------------------------------------------ logo geometry */

/**
 * The one mark, written out.
 *
 * Three equal 20x20 modules on a 28 unit pitch inside a 64 grid - top-left,
 * top-right, bottom-left - plus one 12x12 module in the bottom-right cell,
 * which is what gives the symbol its deliberate asymmetry. The lockup is the
 * same mark translated to x=0 so the wordmark can sit beside it in a 220 grid.
 *
 * `Gamaj.html` at the workspace root is the design source of truth and carries
 * this path. These two strings are that path, and nothing else may serve as a
 * mark.
 */
const MARK_PATH =
	"M8 8h20v20H8V8zm28 0h20v20H36V8zM8 36h20v20H8V36zm28 8h12v12H36V44z";
const LOCKUP_PATH =
	"M0 8h20v20H0V8zm28 0h20v20H28V8zM0 36h20v20H0V36zm28 8h12v12H28V44z";
const CANONICAL_PATHS = new Set([MARK_PATH, LOCKUP_PATH]);
const CANONICAL_VIEWBOXES = new Set(["0 0 64 64", "0 0 220 64"]);

/** The Persian wordmark, written as escapes so this file stays ASCII. */
const PERSIAN_NAME = "\u06af\u0645\u062c";

/**
 * Surfaces rather than assets: the places a person actually meets a Gamaj
 * mark. An entry may be a directory or a single file. Paths are relative to
 * the workspace root.
 */
const LOGO_DIRS = (
	process.env.GAMAJ_LOGO_DIRS ??
	"Panel/dashboard/src,Panel/templates/subscription,Bot/internal,Web/index.html,Web/assets/js"
)
	.split(",")
	.map((entry) => entry.trim())
	.filter(Boolean)
	.map((entry) => path.join(ROOT, entry));

/** Extensions that can carry an inline mark. */
const LOGO_SURFACE_EXT = /\.(html|tsx|jsx|ts|js|mjs|cjs|vue|svelte)$/;

/**
 * What makes an inline `<svg>` the brand rather than an interface icon. The
 * icons sharing these surfaces - a chevron, a shield, a download arrow - carry
 * none of this, so the rule fires on the mark and leaves them alone.
 */
const BRAND_LABEL = new RegExp(
	`aria-label\\s*=\\s*["']\\s*(?:Gamaj|${PERSIAN_NAME})`,
	"i",
);
const BRAND_CLASS = /class\s*=\s*["'][^"']*\b(?:docs-mark|brand-mark|logo-mark|mark)\b/;

/**
 * Shapes the mark cannot be made of. Every module is a filled rectangle, so a
 * circle, an ellipse or a polygon in a mark means something was drawn by hand:
 * that is exactly how the invented mascot reached a shipped panel.
 */
const NOT_A_MODULE = /<(circle|ellipse|polygon|polyline)\b/;

const logoOffenders = [];
let logoSurfaces = 0;
let logoAssets = 0;

/** Every inline `<svg>...</svg>` in a file, with its source offset. */
function inlineSvgs(text) {
	const out = [];
	const open = /<svg\b/g;
	let m;
	while ((m = open.exec(text)) !== null) {
		const end = text.indexOf("</svg>", m.index);
		const stop = end === -1 ? text.length : end + 6;
		out.push({ start: m.index, body: text.slice(m.index, stop) });
		open.lastIndex = stop;
	}
	return out;
}

function checkLogoSurface(p) {
	if (!LOGO_SURFACE_EXT.test(p)) return;
	// Markup inside a comment is not rendered, and these files carry comments
	// that quote the old broken logo exactly so the next reader knows why it
	// was wrong. Scanning those would flag the explanation as the defect.
	const text = fs
		.readFileSync(p, "utf8")
		.replace(/<!--[\s\S]*?-->/g, (block) => block.replace(/[^\n]/g, " "));
	const rel = path.relative(ROOT, p).replace(/\\/g, "/");
	logoSurfaces++;

	for (const { start, body } of inlineSvgs(text)) {
		if (!BRAND_LABEL.test(body) && !BRAND_CLASS.test(body)) continue;
		const line = text.slice(0, start).split("\n").length;

		// A mark with no viewBox scales to whatever box it is dropped into,
		// which is how a 220x64 lockup ends up squashed into a square.
		const viewBox = /\bviewBox\s*=\s*["']([^"']+)["']/.exec(body);
		if (!viewBox) {
			logoOffenders.push(`${rel}:${line} -> brand <svg> has no viewBox`);
		} else if (!CANONICAL_VIEWBOXES.has(viewBox[1].trim())) {
			logoOffenders.push(
				`${rel}:${line} -> brand <svg> viewBox "${viewBox[1]}" is not a Gamaj grid`,
			);
		}

		const curve = NOT_A_MODULE.exec(body);
		if (curve) {
			logoOffenders.push(
				`${rel}:${line} -> brand <svg> contains <${curve[1]}>; the mark is four square modules`,
			);
		}

		// Every literal path must be the mark. A path reached through a
		// constant is allowed, as long as the file actually holds the
		// canonical geometry somewhere.
		const paths = [...body.matchAll(/\bd\s*=\s*["']([^"']+)["']/g)].map(
			(match) => match[1],
		);
		for (const d of paths) {
			if (!CANONICAL_PATHS.has(d)) {
				logoOffenders.push(`${rel}:${line} -> brand path is not the Gamaj symbol`);
			}
		}
		if (paths.length === 0 && !text.includes(MARK_PATH) && !text.includes(LOCKUP_PATH)) {
			logoOffenders.push(
				`${rel}:${line} -> brand <svg> has no path and the file holds no canonical geometry`,
			);
		}
	}

	// The shipped SVGs carry their own prefers-color-scheme fill, because an
	// image rendered outside the document inherits nothing. That makes them
	// right for a README or a tab strip, which follow the reader's operating
	// system, and wrong inside an app whose theme the reader chose in the app.
	// Inside a surface the mark is drawn inline instead.
	const imgTag = /<img\b[^>]*\bsrc\s*=\s*["']([^"']*gamaj[^"']*\.svg)["']/gi;
	let ref;
	while ((ref = imgTag.exec(text)) !== null) {
		const line = text.slice(0, ref.index).split("\n").length;
		logoOffenders.push(
			`${rel}:${line} -> <img src="${ref[1]}"> follows the OS colour scheme, not the app's; draw the mark inline`,
		);
	}
}

for (const LOGO_DIR of LOGO_DIRS) {
	if (!fs.existsSync(LOGO_DIR)) continue;
	if (fs.statSync(LOGO_DIR).isFile()) {
		checkLogoSurface(LOGO_DIR);
		continue;
	}
	const walkLogo = (dir) => {
		for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
			if (entry.name === "node_modules" || entry.name === ".git") continue;
			const p = path.join(dir, entry.name);
			if (entry.isDirectory()) {
				walkLogo(p);
				continue;
			}
			if (entry.isFile()) checkLogoSurface(p);
		}
	};
	walkLogo(LOGO_DIR);
}

/**
 * The shipped asset files. They are the one place a mark may be reached
 * through `<img>`, and they are read by anything that cannot inline: GitHub
 * READMEs, tab strips, pinned tabs. Each must hold the canonical geometry and
 * the colour-scheme block that makes it legible on both dark and light.
 */
const BRAND_ASSETS = new Map([
	["gamaj-mark.svg", MARK_PATH],
	["gamaj-logo.svg", LOCKUP_PATH],
]);

(function walkAssets(dir) {
	for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
		if (SKIP_DIRS.has(entry.name)) continue;
		const p = path.join(dir, entry.name);
		if (entry.isDirectory()) {
			walkAssets(p);
			continue;
		}
		if (!entry.isFile()) continue;
		const want = BRAND_ASSETS.get(entry.name);
		if (!want) continue;
		const rel = path.relative(ROOT, p).replace(/\\/g, "/");
		const text = fs.readFileSync(p, "utf8");
		logoAssets++;
		if (!text.includes(`d="${want}"`)) {
			logoOffenders.push(`${rel} -> path data is not the canonical Gamaj symbol`);
		}
		const viewBox = /\bviewBox\s*=\s*["']([^"']+)["']/.exec(text);
		if (!viewBox || !CANONICAL_VIEWBOXES.has(viewBox[1].trim())) {
			logoOffenders.push(`${rel} -> viewBox is not a Gamaj grid`);
		}
		// Without this block the mark renders black on every dark surface,
		// because currentColor outside a document falls back to its initial
		// value. That is not a style preference, it is an invisible logo.
		if (!/prefers-color-scheme\s*:\s*light/.test(text)) {
			logoOffenders.push(
				`${rel} -> no prefers-color-scheme block, so this file renders black on dark backgrounds`,
			);
		}
	}
})(ROOT);

if (logoOffenders.length > 0) {
	console.error("check-brand-geometry: FAILED (logo geometry)");
	for (const o of logoOffenders) console.error(`  ${o}`);
	console.error(
		"There is one mark: three equal 20x20 modules on a 28 unit pitch,\n" +
			"plus one 12x12 module in the bottom-right cell. Inside an app,\n" +
			"draw it inline so it follows the theme the reader chose there.\n" +
			"Outside the document - a README or a tab strip - ship the\n" +
			"gamaj-mark.svg / gamaj-logo.svg asset, which carries its own\n" +
			"prefers-color-scheme fill.",
	);
	process.exit(1);
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
		`${paletteScanned} Gamaj surface sources on the palette; ` +
		`${logoSurfaces} surfaces and ${logoAssets} brand assets on the canonical mark)`,
);
