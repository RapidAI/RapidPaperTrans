#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
PDF translation overlay script using PyMuPDF.
Reads translated blocks from JSON and overlays them on the original PDF.
"""

import sys
import json
import os

try:
    import pymupdf
except ImportError:
    try:
        import fitz as pymupdf
    except ImportError:
        print("Error: PyMuPDF not installed. Install with: pip install PyMuPDF", file=sys.stderr)
        sys.exit(1)

WHITE = pymupdf.pdfcolor["white"]
FIXED_FONT_SIZE = 10


def write_translations(input_pdf, translations_json, output_pdf):
    """
    Overlay translations on PDF.
    
    Input JSON format:
    [
        {
            "id": "page_0_block_0",
            "page": 0,
            "x": 72.0,
            "y": 720.0,
            "width": 468.0,
            "height": 50.0,
            "text": "Original text...",
            "translated_text": "翻译后的文本..."
        },
        ...
    ]
    """
    # Load translations
    with open(translations_json, 'r', encoding='utf-8') as f:
        blocks = json.load(f)
    
    doc = pymupdf.open(input_pdf)
    translated_count = 0
    skipped_count = 0
    
    for block in blocks:
        # Skip blocks without translation
        translated_text = block.get("translated_text", "").strip()
        if not translated_text:
            skipped_count += 1
            continue
        
        page_num = block.get("page", 0)
        if page_num >= len(doc):
            continue
        
        page = doc[page_num]
        
        # Get bounding box
        x = block.get("x", 0)
        y = block.get("y", 0)
        width = block.get("width", 100)
        height = block.get("height", 20)
        
        rect = pymupdf.Rect(x, y, x + width, y + height)
        
        # Ensure rect is within page bounds
        rect = rect & page.rect
        if rect.is_empty:
            continue
        
        # Cover original text with white
        page.draw_rect(rect, color=None, fill=WHITE)
        
        # Insert translation with fixed font size
        try:
            rc = page.insert_textbox(
                rect,
                translated_text,
                fontsize=FIXED_FONT_SIZE,
                fontname="china-s",
                align=pymupdf.TEXT_ALIGN_LEFT
            )
            if rc < 0:
                # Text didn't fit, try smaller font
                page.insert_textbox(
                    rect,
                    translated_text,
                    fontsize=FIXED_FONT_SIZE - 2,
                    fontname="china-s",
                    align=pymupdf.TEXT_ALIGN_LEFT
                )
        except Exception as e:
            # Fallback to htmlbox
            try:
                html_text = f'<p style="font-size:{FIXED_FONT_SIZE}pt;">{translated_text}</p>'
                page.insert_htmlbox(rect, html_text)
            except:
                try:
                    page.insert_htmlbox(rect, translated_text)
                except:
                    print(f"Warning: Failed to insert text at page {page_num + 1}: {e}", file=sys.stderr)
                    continue
        
        translated_count += 1
    
    # Optimize and save
    try:
        doc.subset_fonts()
    except:
        pass
    
    doc.ez_save(output_pdf)
    doc.close()
    
    print(f"Translated: {translated_count} blocks")
    print(f"Skipped: {skipped_count} blocks (no translation)")
    print(f"Output: {output_pdf}")
    
    return True


def main():
    if len(sys.argv) < 4:
        print("Usage: write_translations.py <input_pdf> <translations_json> <output_pdf>", file=sys.stderr)
        sys.exit(1)
    
    input_pdf = sys.argv[1]
    translations_json = sys.argv[2]
    output_pdf = sys.argv[3]
    
    if not os.path.exists(input_pdf):
        print(f"Error: PDF not found: {input_pdf}", file=sys.stderr)
        sys.exit(1)
    
    if not os.path.exists(translations_json):
        print(f"Error: Translations JSON not found: {translations_json}", file=sys.stderr)
        sys.exit(1)
    
    success = write_translations(input_pdf, translations_json, output_pdf)
    sys.exit(0 if success else 1)


if __name__ == '__main__':
    main()
