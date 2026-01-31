#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
PDF translation overlay script using PyMuPDF.
Can work in two modes:
1. With pre-translated data (translations JSON)
2. With API config (calls translation API directly)

Based on: https://medium.com/@pymupdf/translating-pdfs-a-practical-pymupdf-guide-c1c54b024042
"""

import sys
import json
import os
import re
import hashlib

try:
    import pymupdf
except ImportError:
    try:
        import fitz as pymupdf
    except ImportError:
        print("Error: PyMuPDF not installed. Install with: pip install PyMuPDF")
        sys.exit(1)

WHITE = pymupdf.pdfcolor["white"]


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


def should_translate(text):
    """Determine if text should be translated"""
    text = text.strip()
    
    if not text or len(text) < 3:
        return False
    
    if is_line_number(text):
        return False
    
    if is_formula(text):
        return False
    
    # Skip if mostly numbers/symbols
    alpha_count = sum(1 for c in text if c.isalpha())
    if len(text) > 0 and alpha_count / len(text) < 0.3:
        return False
    
    return True


class TranslationCache:
    """Simple file-based translation cache"""
    def __init__(self, cache_path=None):
        self.cache_path = cache_path
        self.cache = {}
        if cache_path:
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
            with open(self.cache_path, 'w', encoding='utf-8') as f:
                json.dump(self.cache, f, ensure_ascii=False, indent=2)
    
    def get(self, text):
        key = hashlib.md5(text.encode()).hexdigest()
        return self.cache.get(key)
    
    def set(self, text, translation):
        key = hashlib.md5(text.encode()).hexdigest()
        self.cache[key] = translation
    
    def load_from_translations(self, translations):
        """Load translations from JSON array"""
        for t in translations:
            orig_text = t.get('text', '').strip()
            trans_text = t.get('translated_text', '').strip()
            if orig_text and trans_text:
                self.set(orig_text, trans_text)


def translate_text_api(text, api_key, base_url, model):
    """Translate text using OpenAI-compatible API"""
    try:
        import requests
    except ImportError:
        print("Warning: requests not installed, cannot call API")
        return None
    
    headers = {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json"
    }
    
    prompt = f"""Translate the following English text to Chinese. 
Keep any technical terms, formulas, or proper nouns unchanged.
Only output the translation, nothing else.

Text to translate:
{text}"""
    
    data = {
        "model": model,
        "messages": [
            {"role": "user", "content": prompt}
        ]
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
        print(f"Translation API error: {e}")
        return None


def overlay_translations(original_pdf, translations_source, output_pdf, 
                         api_key=None, base_url=None, model=None, cache_path=None):
    """
    Overlay translations on PDF.
    
    Args:
        original_pdf: Path to original PDF
        translations_source: Path to JSON file with translations, or None to use API
        output_pdf: Path to output PDF
        api_key: API key for translation (optional)
        base_url: API base URL (optional)
        model: Model name (optional)
        cache_path: Path to cache file (optional)
    """
    # Setup cache
    cache = TranslationCache(cache_path)
    
    # Load pre-translated data if provided
    if translations_source and os.path.isfile(translations_source):
        with open(translations_source, 'r', encoding='utf-8') as f:
            translations = json.load(f)
        cache.load_from_translations(translations)
        print(f"Loaded {len(translations)} pre-translated blocks")
    
    use_api = api_key and base_url and model
    
    doc = pymupdf.open(original_pdf)
    blocks_processed = 0
    blocks_skipped = 0
    blocks_from_cache = 0
    blocks_from_api = 0
    
    print(f"Processing {len(doc)} pages...")
    
    for page_num in range(len(doc)):
        page = doc[page_num]
        
        # Extract text blocks using PyMuPDF
        blocks = page.get_text("blocks", flags=pymupdf.TEXT_DEHYPHENATE)
        
        for block in blocks:
            if len(block) < 5:
                continue
            
            bbox = block[:4]
            text = block[4] if isinstance(block[4], str) else ""
            text = text.strip()
            
            if not should_translate(text):
                blocks_skipped += 1
                continue
            
            # Try to get translation from cache
            translation = cache.get(text)
            
            if translation:
                blocks_from_cache += 1
            elif use_api:
                # Call API for translation
                translation = translate_text_api(text, api_key, base_url, model)
                if translation:
                    cache.set(text, translation)
                    blocks_from_api += 1
            
            if not translation:
                continue
            
            # Apply translation
            rect = pymupdf.Rect(bbox)
            
            # Cover original text with white
            page.draw_rect(rect, color=None, fill=WHITE)
            
            # Insert translation using htmlbox
            page.insert_htmlbox(rect, translation)
            blocks_processed += 1
        
        print(f"  Page {page_num + 1}: done")
    
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
    print(f"Processed: {blocks_processed} blocks")
    print(f"From cache: {blocks_from_cache}")
    print(f"From API: {blocks_from_api}")
    print(f"Skipped: {blocks_skipped} (formula/symbol)")
    print(f"Output: {output_pdf}")
    return True


def main():
    if len(sys.argv) < 4:
        print("Usage: overlay_pdf.py <original_pdf> <translations_json> <output_pdf>")
        print("")
        print("Or with API (set environment variables):")
        print("  OPENAI_API_KEY  - API key")
        print("  OPENAI_BASE_URL - API base URL")
        print("  OPENAI_MODEL    - Model name")
        sys.exit(1)
    
    original_pdf = sys.argv[1]
    translations_json = sys.argv[2]
    output_pdf = sys.argv[3]
    
    # Check for API config in environment
    api_key = os.environ.get("OPENAI_API_KEY")
    base_url = os.environ.get("OPENAI_BASE_URL")
    model = os.environ.get("OPENAI_MODEL")
    
    # Cache path (same directory as output)
    cache_path = os.path.splitext(output_pdf)[0] + "_cache.json"
    
    if not os.path.exists(original_pdf):
        print(f"Error: Original PDF not found: {original_pdf}")
        sys.exit(1)
    
    success = overlay_translations(
        original_pdf, 
        translations_json, 
        output_pdf,
        api_key=api_key,
        base_url=base_url,
        model=model,
        cache_path=cache_path
    )
    sys.exit(0 if success else 1)


if __name__ == '__main__':
    main()
