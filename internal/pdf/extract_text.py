#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
PDF text extraction script using PyMuPDF.
Extracts text blocks with position information for Go to translate.
"""

import sys
import json
import os
import re

try:
    import pymupdf
except ImportError:
    try:
        import fitz as pymupdf
    except ImportError:
        print("Error: PyMuPDF not installed. Install with: pip install PyMuPDF", file=sys.stderr)
        sys.exit(1)


def is_formula(text):
    """Check if text is a mathematical formula"""
    # Strong math symbols that clearly indicate formulas
    strong_math_symbols = "∫∑∏√∂∇"
    
    # Check for strong math symbols
    if any(c in text for c in strong_math_symbols):
        return True
    
    # Check for LaTeX commands
    if re.search(r'\\[a-zA-Z]+', text):
        return True
    
    # Pattern like (5) or (A.1) at end - equation number (only if short)
    if re.search(r'^\s*\(\d+\)\s*$|^\s*\([A-Z]\.\d+\)\s*$', text.strip()):
        return True
    
    # Check for subscript/superscript patterns (but only if text is short and has many)
    if len(text) < 50:
        subscript_count = len(re.findall(r'[a-zA-Z][_^]\d', text))
        if subscript_count > 2:
            return True
    
    # Count math-related characters (but be more lenient)
    math_symbols = "±×÷≤≥≠≈∞∈∉⊂⊃∪∩∧∨¬∀∃αβγδεζηθικλμνξοπρστυφχψωΓΔΘΛΞΠΣΦΨΩ"
    math_count = sum(1 for c in text if c in math_symbols)
    
    # Only mark as formula if more than 40% are special math symbols (not including common punctuation)
    if len(text) > 0 and math_count / len(text) > 0.4:
        return True
    
    return False


def is_line_number(text):
    """Check if text is just line numbers"""
    lines = text.strip().split('\n')
    if len(lines) < 3:
        return False
    number_lines = sum(1 for line in lines if line.strip().isdigit())
    return number_lines / len(lines) > 0.7


def should_translate(text):
    """Determine if text should be translated"""
    text = text.strip()
    
    if not text or len(text) < 3:
        return False
    
    if is_line_number(text):
        return False
    
    if is_formula(text):
        return False
    
    alpha_count = sum(1 for c in text if c.isalpha())
    if len(text) > 0 and alpha_count / len(text) < 0.3:
        return False
    
    # Skip if already contains Chinese
    chinese_count = sum(1 for c in text if '\u4e00' <= c <= '\u9fff')
    if chinese_count > len(text) * 0.3:
        return False
    
    return True


def extract_text_blocks(pdf_path, output_json):
    """
    Extract text blocks from PDF and save to JSON.
    
    Output format:
    [
        {
            "id": "page_0_block_0",
            "page": 0,
            "x": 72.0,
            "y": 720.0,
            "width": 468.0,
            "height": 50.0,
            "text": "Original text...",
            "block_type": "text"  # or "formula"
        },
        ...
    ]
    """
    doc = pymupdf.open(pdf_path)
    blocks_data = []
    block_id = 0
    page_count = len(doc)
    
    for page_num in range(page_count):
        page = doc[page_num]
        blocks = page.get_text("blocks", flags=pymupdf.TEXT_DEHYPHENATE)
        
        for block in blocks:
            if len(block) < 5:
                continue
            
            bbox = block[:4]
            text = block[4] if isinstance(block[4], str) else ""
            text = text.strip()
            
            if not text:
                continue
            
            # Determine block type
            block_type = "formula" if is_formula(text) else "text"
            
            # Check if should translate
            translatable = should_translate(text)
            
            block_data = {
                "id": f"page_{page_num}_block_{block_id}",
                "page": page_num,
                "x": bbox[0],
                "y": bbox[1],
                "width": bbox[2] - bbox[0],
                "height": bbox[3] - bbox[1],
                "text": text,
                "block_type": block_type,
                "translatable": translatable
            }
            blocks_data.append(block_data)
            block_id += 1
    
    doc.close()
    
    # Save to JSON
    with open(output_json, 'w', encoding='utf-8') as f:
        json.dump(blocks_data, f, ensure_ascii=False, indent=2)
    
    print(f"Extracted {len(blocks_data)} blocks from {page_count} pages")
    print(f"Output: {output_json}")
    
    return True


def main():
    if len(sys.argv) < 3:
        print("Usage: extract_text.py <input_pdf> <output_json>", file=sys.stderr)
        sys.exit(1)
    
    pdf_path = sys.argv[1]
    output_json = sys.argv[2]
    
    if not os.path.exists(pdf_path):
        print(f"Error: PDF not found: {pdf_path}", file=sys.stderr)
        sys.exit(1)
    
    success = extract_text_blocks(pdf_path, output_json)
    sys.exit(0 if success else 1)


if __name__ == '__main__':
    main()
