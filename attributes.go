package asciidoctor

import (
	"strings"
	"time"
)

// The setters below set well-known document attributes and return the map,
// so they chain:
//
//	attrs := asciidoctor.Attributes{}.TableOfContents(asciidoctor.PlacementLeft).SectionNumbers(true)
//
// A boolean flag set to false unsets the attribute.

// Placement is where the table of contents goes (the toc attribute).
type Placement string

const (
	PlacementAuto     Placement = "auto"
	PlacementTop      Placement = "top"
	PlacementBottom   Placement = "bottom"
	PlacementLeft     Placement = "left"
	PlacementRight    Placement = "right"
	PlacementPreamble Placement = "preamble"
	PlacementMacro    Placement = "macro"
)

// Values of the icons attribute.
const (
	// IconsImage uses image icons (the icons attribute set, but empty).
	IconsImage = ""
	// IconsFont uses Font Awesome icons.
	IconsFont = "font"
)

// CompatMode selects AsciiDoc compatibility mode.
type CompatMode int

const (
	// CompatModeDefault is modern AsciiDoc (compat-mode unset).
	CompatModeDefault CompatMode = iota
	// CompatModeLegacy follows AsciiDoc.py's legacy syntax rules.
	CompatModeLegacy
)

func (a Attributes) flag(name string, on bool) Attributes {
	if on {
		a[name] = ""
	} else {
		a[name] = nil
	}
	return a
}

// Set sets an attribute to a value (see Attributes for value conversion).
func (a Attributes) Set(name string, value any) Attributes { a[name] = value; return a }

// Unset unsets an attribute (it is passed as name!), overriding the document.
func (a Attributes) Unset(name string) Attributes { delete(a, name); a[name+"!"] = ""; return a }

// Arguments sets attributes from "name=value" or "name" items; items may also
// be given as one space-separated string, as on the command line.
func (a Attributes) Arguments(items ...string) Attributes {
	for _, item := range items {
		for _, field := range strings.Fields(item) {
			if name, value, ok := strings.Cut(field, "="); ok {
				a[name] = value
			} else {
				a[field] = ""
			}
		}
	}
	return a
}

func (a Attributes) AllowURIRead(on bool) Attributes      { return a.flag("allow-uri-read", on) }
func (a Attributes) AttributeMissing(v string) Attributes { return a.Set("attribute-missing", v) }
func (a Attributes) AttributeUndefined(v string) Attributes {
	return a.Set("attribute-undefined", v)
}
func (a Attributes) Backend(v string) Attributes           { return a.Set("backend", v) }
func (a Attributes) Title(v string) Attributes             { return a.Set("title", v) }
func (a Attributes) Doctype(v string) Attributes           { return a.Set("doctype", v) }
func (a Attributes) ImagesDir(v string) Attributes         { return a.Set("imagesdir", v) }
func (a Attributes) SourceLanguage(v string) Attributes    { return a.Set("source-language", v) }
func (a Attributes) SourceHighlighter(v string) Attributes { return a.Set("source-highlighter", v) }
func (a Attributes) MaxIncludeDepth(n int) Attributes      { return a.Set("max-include-depth", n) }
func (a Attributes) SectNumLevels(n int) Attributes        { return a.Set("sectnumlevels", n) }
func (a Attributes) TOCLevels(n int) Attributes            { return a.Set("toclevels", n) }
func (a Attributes) Hardbreaks(on bool) Attributes         { return a.flag("hardbreaks", on) }
func (a Attributes) CacheURI(on bool) Attributes           { return a.flag("cache-uri", on) }
func (a Attributes) HideURIScheme(on bool) Attributes      { return a.flag("hide-uri-scheme", on) }
func (a Attributes) AppendixCaption(v string) Attributes   { return a.Set("appendix-caption", v) }

// Math sets the STEM notation ("asciimath" or "latexmath").
func (a Attributes) Math(v string) Attributes           { return a.Set("stem", v) }
func (a Attributes) SkipFrontMatter(on bool) Attributes { return a.flag("skip-front-matter", on) }

// SectAnchors adds anchors to section titles (AsciidoctorJ's setAnchors).
func (a Attributes) SectAnchors(on bool) Attributes    { return a.flag("sectanchors", on) }
func (a Attributes) UntitledLabel(v string) Attributes { return a.Set("untitled-label", v) }

// IgnoreUndefinedAttributes keeps references to undefined attributes.
func (a Attributes) IgnoreUndefinedAttributes(on bool) Attributes {
	return a.flag("ignore-undefined", on)
}

// TableOfContents enables the table of contents at placement.
func (a Attributes) TableOfContents(placement Placement) Attributes {
	return a.Set("toc", string(placement))
}

// TOC enables (with the default placement) or disables the table of contents.
func (a Attributes) TOC(on bool) Attributes {
	if on {
		a["toc"] = ""
	} else {
		delete(a, "toc")
	}
	return a
}

// ShowTitle shows (showtitle) or hides (notitle) the document title.
func (a Attributes) ShowTitle(show bool) Attributes {
	if show {
		a["showtitle"] = true
		delete(a, "notitle")
	} else {
		a["notitle"] = true
		delete(a, "showtitle")
	}
	return a
}

// LocalDate sets localdate (as yyyy-mm-dd).
func (a Attributes) LocalDate(t time.Time) Attributes {
	return a.Set("localdate", t.Format("2006-01-02"))
}

// LocalTime sets localtime (as HH:MM:SS zone).
func (a Attributes) LocalTime(t time.Time) Attributes {
	return a.Set("localtime", t.Format("15:04:05 MST"))
}

// DocDate sets docdate (as yyyy-mm-dd).
func (a Attributes) DocDate(t time.Time) Attributes { return a.Set("docdate", t.Format("2006-01-02")) }

// DocTime sets doctime (as HH:MM:SS zone).
func (a Attributes) DocTime(t time.Time) Attributes {
	return a.Set("doctime", t.Format("15:04:05 MST"))
}

func (a Attributes) StylesheetName(v string) Attributes { return a.Set("stylesheet", v) }

// UnsetStylesheet disables the stylesheet.
func (a Attributes) UnsetStylesheet() Attributes       { return a.flag("stylesheet!", true) }
func (a Attributes) StylesDir(v string) Attributes     { return a.Set("stylesdir", v) }
func (a Attributes) LinkCSS(on bool) Attributes        { return a.flag("linkcss", on) }
func (a Attributes) CopyCSS(on bool) Attributes        { return a.flag("copycss", on) }
func (a Attributes) Icons(v string) Attributes         { return a.Set("icons", v) }
func (a Attributes) IconFontRemote(on bool) Attributes { return a.flag("iconfont-remote", on) }
func (a Attributes) IconFontCDN(url string) Attributes { return a.Set("iconfont-cdn", url) }
func (a Attributes) IconFontName(v string) Attributes  { return a.Set("iconfont-name", v) }
func (a Attributes) IconsDir(v string) Attributes      { return a.Set("iconsdir", v) }
func (a Attributes) DataURI(on bool) Attributes        { return a.flag("data-uri", on) }
func (a Attributes) SectionNumbers(on bool) Attributes { return a.flag("sectnums", on) }
func (a Attributes) LinkAttrs(on bool) Attributes      { return a.flag("linkattrs", on) }
func (a Attributes) Experimental(on bool) Attributes   { return a.flag("experimental", on) }
func (a Attributes) NoFooter(on bool) Attributes       { return a.flag("nofooter", on) }

// CompatMode sets compatibility mode. Asciidoctor enables it whenever the
// compat-mode attribute is present, so CompatModeDefault removes it.
func (a Attributes) CompatMode(m CompatMode) Attributes {
	if m == CompatModeLegacy {
		a["compat-mode"] = "legacy"
	} else {
		delete(a, "compat-mode")
	}
	return a
}
