#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Complete PDF translation script using PyMuPDF.
Extracts text, calls translation API, and overlays translations.

This script does everything in one pass:
1. Extract text blocks using PyMuPDF
2. Translate each block via API
3. Overlay translations on the original PDF

Based on: https://medium.com/@pymupdf/translating-pdfs-a-practical-pymupdf-guide-c1c54b024042
"""

import sys
import json
import os
import hashlib
import re

try:
    import requests
except ImportError:
    print("Error: requests not installed. Install with: pip install requests")
    sys.exit(1)

try:
    import pymupdf
except ImportError:
    try:
        import fitz as pymupdf
    except ImportError:
        print("Error: PyMuPDF not installed. Install with: pip install PyMuPDF")
        sys.exit(1)

WHITE = pymupdf.pdfcolor["white"]

# Base font size - will be adjusted based on original text
BASE_FONT_SIZE = 12  # Increased from 10 to 12 for better readability


class TranslationCache:
    """Simple file-based translation cache"""
    def __init__(self, cache_path):
        self.cache_path = cache_path
        self.cache = {}
        self.load()
    
    def load(self):
        if self.cache_path and os.path.exists(self.cache_path):
            try:
                with open(self.cache_path, 'r', encoding='utf-8') as f:
                    self.cache = json.load(f)
            except:
                self.cache = {}
    
    def save(self):
        if self.cache_path:
            try:
                with open(self.cache_path, 'w', encoding='utf-8') as f:
                    json.dump(self.cache, f, ensure_ascii=False, indent=2)
            except Exception as e:
                print(f"Warning: Failed to save cache: {e}")
    
    def get(self, text):
        key = hashlib.md5(text.encode()).hexdigest()
        return self.cache.get(key)
    
    def set(self, text, translation):
        key = hashlib.md5(text.encode()).hexdigest()
        self.cache[key] = translation


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
    
    # Only mark as formula if more than 40% are special math symbols
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


def is_reference(text):
    """Check if text is a reference/citation"""
    text_lower = text.lower().strip()
    # Check for common reference patterns
    if text_lower.startswith('[') and ']' in text_lower[:10]:
        return True
    return False


def should_translate(text):
    """Determine if text should be translated"""
    text = text.strip()
    
    # Skip empty or very short text
    if not text or len(text) < 3:
        return False
    
    # Skip line numbers
    if is_line_number(text):
        return False
    
    # Skip formulas
    if is_formula(text):
        return False
    
    # Skip if mostly numbers/symbols (less than 30% alphabetic)
    alpha_count = sum(1 for c in text if c.isalpha())
    if len(text) > 0 and alpha_count / len(text) < 0.3:
        return False
    
    # Skip if already contains Chinese (already translated)
    chinese_count = sum(1 for c in text if '\u4e00' <= c <= '\u9fff')
    if chinese_count > len(text) * 0.3:
        return False
    
    return True


def translate_text(text, api_key, base_url, model):
    """Translate text using OpenAI-compatible API"""
    headers = {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json"
    }
    
    prompt = f"""Translate the following English text to Chinese. 
Keep any technical terms, formulas, numbers, or proper nouns unchanged.
Only output the translation, nothing else.

Text to translate:
{text}"""
    
    data = {
        "model": model,
        "messages": [
            {"role": "user", "content": prompt}
        ],
        "temperature": 0.3
    }
    
    try:
        response = requests.post(
            f"{base_url}/chat/completions",
            headers=headers,
            json=data,
            timeout=60
        )
        response.raise_for_status()
        result = response.json()
        return result["choices"][0]["message"]["content"].strip()
    except Exception as e:
        print(f"Translation error: {e}")
        return None


def translate_pdf(input_pdf, output_pdf, api_key, base_url, model, cache_path=None):
    """
    Translate PDF using PyMuPDF's official approach.
    
    1. Extract text blocks with get_text("dict") to get font info
    2. Translate each block
    3. Cover original with white rectangle
    4. Insert translation with appropriate font size and rotation
    """
    # Setup cache
    cache = TranslationCache(cache_path) if cache_path else TranslationCache(None)
    
    doc = pymupdf.open(input_pdf)
    total_blocks = 0
    translated_blocks = 0
    cached_blocks = 0
    skipped_blocks = 0
    
    print(f"Processing {len(doc)} pages...")
    
    for page_num in range(len(doc)):
        page = doc[page_num]
        page_width = page.rect.width
        page_height = page.rect.height
        
        # Extract text with detailed info including font size
        text_dict = page.get_text("dict", flags=pymupdf.TEXT_DEHYPHENATE)
        
        # Process each block
        for block in text_dict.get("blocks", []):
            if block.get("type") != 0:  # Not a text block
                continue
            
            bbox = block.get("bbox")
            if not bbox:
                continue
            
            # Extract text and font info from lines
            text_parts = []
            font_sizes = []
            
            for line in block.get("lines", []):
                for span in line.get("spans", []):
                    span_text = span.get("text", "").strip()
                    if span_text:
                        text_parts.append(span_text)
                        font_sizes.append(span.get("size", BASE_FONT_SIZE))
            
            if not text_parts:
                continue
            
            text = " ".join(text_parts)
            
            if not should_translate(text):
                skipped_blocks += 1
                continue
            
            total_blocks += 1
            
            # Check cache
            translation = cache.get(text)
            if translation:
                cached_blocks += 1
            else:
                # Translate
                translation = translate_text(text, api_key, base_url, model)
                if translation:
                    cache.set(text, translation)
            
            if not translation:
                continue
            
            # Calculate appropriate font size
            # Use average of original font sizes, but ensure readability
            avg_font_size = sum(font_sizes) / len(font_sizes) if font_sizes else BASE_FONT_SIZE
            # Chinese text needs slightly larger size for readability
            font_size = max(BASE_FONT_SIZE, avg_font_size * 1.1)
            
            # Apply translation
            rect = pymupdf.Rect(bbox)
            
            # Ensure rect is within page bounds
            rect = rect & page.rect
            if rect.is_empty:
                continue
            
            # Cover original text with white
            page.draw_rect(rect, color=None, fill=WHITE)
            
            # Insert translation with calculated font size
            try:
                # Try using built-in Chinese font first
                rc = page.insert_textbox(
                    rect,
                    translation,
                    fontsize=font_size,
                    fontname="china-s",  # Built-in Chinese font
                    align=pymupdf.TEXT_ALIGN_LEFT,
                    rotate=0  # Ensure horizontal text
                )
                if rc < 0:
                    # Text didn't fit, try with smaller font
                    font_size = font_size * 0.85
                    rc = page.insert_textbox(
                        rect,
                        translation,
                        fontsize=font_size,
                        fontname="china-s",
                        align=pymupdf.TEXT_ALIGN_LEFT,
                        rotate=0
                    )
                    if rc < 0:
                        # Still doesn't fit, try even smaller
                        font_size = BASE_FONT_SIZE * 0.9
                        page.insert_textbox(
                            rect,
                            translation,
                            fontsize=font_size,
                            fontname="china-s",
                            align=pymupdf.TEXT_ALIGN_LEFT,
                            rotate=0
                        )
            except Exception as e:
                # Fallback to htmlbox if textbox fails
                try:
                    # Use HTML with explicit font size
                    html_text = f'<p style="font-size:{font_size}pt;font-family:sans-serif;margin:0;padding:0;">{translation}</p>'
                    page.insert_htmlbox(rect, html_text)
                except:
                    print(f"Warning: Failed to insert text at page {page_num + 1}: {e}")
                    continue
            
            translated_blocks += 1
        
        print(f"  Page {page_num + 1}: {translated_blocks} blocks translated")
    
    # Save cache
    cache.save()
    
    # Optimize and save
    try:
        doc.subset_fonts()
    except:
        pass
    
    doc.ez_save(output_pdf)
    doc.close()
    
    print(f"\n=== Translation Complete ===")
    print(f"Total translatable blocks: {total_blocks}")
    print(f"Translated: {translated_blocks}")
    print(f"From cache: {cached_blocks}")
    print(f"Skipped (formula/symbol): {skipped_blocks}")
    print(f"Output: {output_pdf}")
    
    return True


def main():
    if len(sys.argv) < 3:
        print("Usage: translate_pdf.py <input_pdf> <output_pdf> [cache_path]")
        print("")
        print("Environment variables:")
        print("  OPENAI_API_KEY  - API key (required)")
        print("  OPENAI_BASE_URL - API base URL (default: https://api.openai.com/v1)")
        print("  OPENAI_MODEL    - Model name (default: gpt-4)")
        sys.exit(1)
    
    input_pdf = sys.argv[1]
    output_pdf = sys.argv[2]
    cache_path = sys.argv[3] if len(sys.argv) > 3 else None
    
    api_key = os.environ.get("OPENAI_API_KEY")
    base_url = os.environ.get("OPENAI_BASE_URL", "https://api.openai.com/v1")
    model = os.environ.get("OPENAI_MODEL", "gpt-4")
    
    if not api_key:
        print("Error: OPENAI_API_KEY environment variable not set")
        sys.exit(1)
    
    if not os.path.exists(input_pdf):
        print(f"Error: Input PDF not found: {input_pdf}")
        sys.exit(1)
    
    print(f"API: {base_url}")
    print(f"Model: {model}")
    print(f"Input: {input_pdf}")
    print(f"Output: {output_pdf}")
    if cache_path:
        print(f"Cache: {cache_path}")
    print("")
    
    success = translate_pdf(input_pdf, output_pdf, api_key, base_url, model, cache_path)
    sys.exit(0 if success else 1)


if __name__ == '__main__':
    main()
