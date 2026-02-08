// Package pdf provides embedded Python scripts for PDF translation.
package pdf

// PyMuPDFTranslateScript is a complete PDF translation script using PyMuPDF
// It extracts text, calls translation API, and overlays translations - all in Python
// This version uses DocLayout-YOLO for layout detection to preserve formulas
const PyMuPDFTranslateScript = `#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
Complete PDF translation using PyMuPDF with DocLayout-YOLO layout detection.
Detects layout first, only translates text regions, preserves formulas.
"""
import sys
import json
import os
import re
import hashlib
import time
import tempfile

try:
    import requests
except ImportError:
    print("Error: requests not installed", file=sys.stderr)
    sys.exit(1)

try:
    import pymupdf
except ImportError:
    try:
        import fitz as pymupdf
    except ImportError:
        print("Error: PyMuPDF not installed", file=sys.stderr)
        sys.exit(1)

# Try to import layout detection (optional)
LAYOUT_DETECTION_AVAILABLE = False
try:
    import onnxruntime as ort
    import cv2
    import numpy as np
    LAYOUT_DETECTION_AVAILABLE = True
except ImportError:
    pass

WHITE = pymupdf.pdfcolor["white"]

# Global font buffer for SimSun (宋体)
SIMSUN_FONT_BUFFER = None

def load_simsun_font():
    """Load SimSun font from system fonts directory."""
    global SIMSUN_FONT_BUFFER
    if SIMSUN_FONT_BUFFER is not None:
        return SIMSUN_FONT_BUFFER
    
    # Try to load SimSun from Windows fonts
    font_paths = [
        "C:/Windows/Fonts/simsun.ttc",
        "C:/Windows/Fonts/SimSun.ttc",
        "/usr/share/fonts/truetype/simsun.ttc",
        "/System/Library/Fonts/SimSun.ttc",
    ]
    
    for path in font_paths:
        if os.path.exists(path):
            try:
                font = pymupdf.Font(fontfile=path)
                SIMSUN_FONT_BUFFER = font.buffer
                print(f"Loaded SimSun font from {path}", file=sys.stderr)
                return SIMSUN_FONT_BUFFER
            except Exception as e:
                print(f"Failed to load font from {path}: {e}", file=sys.stderr)
    
    print("SimSun font not found, using built-in china-s", file=sys.stderr)
    return None

# D4LA model classes (27 classes)
D4LA_CLASS_NAMES = {
    0: "DocTitle", 1: "ParaTitle", 2: "ParaText", 3: "ListText", 4: "RegionTitle",
    5: "Date", 6: "LetterHead", 7: "LetterDear", 8: "LetterSign", 9: "Question",
    10: "OtherText", 11: "RegionKV", 12: "RegionList", 13: "Abstract", 14: "Author",
    15: "TableName", 16: "Table", 17: "Figure", 18: "FigureName", 19: "Equation",
    20: "Reference", 21: "Footer", 22: "PageHeader", 23: "PageFooter", 24: "Number",
    25: "Catalog", 26: "PageNumber"
}

# Classes to translate (text content)
TRANSLATE_CLASS_IDS = {0, 1, 2, 3, 4, 9, 10, 13}  # DocTitle, ParaTitle, ParaText, ListText, RegionTitle, Question, OtherText, Abstract
# Note: Removed TableName (15), FigureName (18), Reference (20) to avoid translating figure/table captions and references

# Classes to skip (non-text or should preserve)
SKIP_CLASS_IDS = {16, 17, 19, 21, 22, 23, 26}  # Table, Figure, Equation, Footer, PageHeader, PageFooter, PageNumber


class LayoutDetector:
    """ONNX-based layout detector using ONNX Runtime."""
    
    def __init__(self, model_path=None):
        self.session = None
        self.model_path = model_path or os.environ.get("DOCLAYOUT_MODEL_PATH")
        self.available = LAYOUT_DETECTION_AVAILABLE
        self.input_size = None  # Will be detected from model
        
    def load_model(self):
        """Load ONNX model using ONNX Runtime."""
        if not self.available:
            return False
        if self.session is not None:
            return True
        
        if not self.model_path:
            print("Error: DOCLAYOUT_MODEL_PATH not set", file=sys.stderr)
            self.available = False
            return False
        
        if not os.path.exists(self.model_path):
            print(f"Error: Model not found: {self.model_path}", file=sys.stderr)
            self.available = False
            return False
        
        try:
            print(f"Loading DocLayout-YOLO ONNX model from {self.model_path}...", file=sys.stderr)
            # Use CPU provider for compatibility
            self.session = ort.InferenceSession(self.model_path, providers=['CPUExecutionProvider'])
            
            # Auto-detect input size from model metadata
            input_size = 1024  # Default
            try:
                # Try to get imgsz from model metadata
                meta = self.session.get_modelmeta()
                if meta and meta.custom_metadata_map:
                    imgsz_str = meta.custom_metadata_map.get('imgsz', '')
                    if imgsz_str:
                        # Parse [1600, 1600] format
                        import ast
                        imgsz = ast.literal_eval(imgsz_str)
                        if isinstance(imgsz, (list, tuple)) and len(imgsz) >= 1:
                            input_size = imgsz[0]
            except:
                pass
            self.input_size = input_size
            print(f"Model input size: {self.input_size}", file=sys.stderr)
            
            print("Model loaded.", file=sys.stderr)
            return True
        except Exception as e:
            print(f"Error loading model: {e}", file=sys.stderr)
            self.available = False
            return False
    
    def preprocess_image(self, image_path):
        """Preprocess image for YOLO model."""
        # Read image
        img = cv2.imread(image_path)
        if img is None:
            raise ValueError(f"Failed to read image: {image_path}")
        
        # Get original size
        orig_h, orig_w = img.shape[:2]
        
        # Resize to model input size (letterbox)
        scale = min(self.input_size / orig_w, self.input_size / orig_h)
        new_w = int(orig_w * scale)
        new_h = int(orig_h * scale)
        
        resized = cv2.resize(img, (new_w, new_h), interpolation=cv2.INTER_LINEAR)
        
        # Create padded image
        padded = np.full((self.input_size, self.input_size, 3), 114, dtype=np.uint8)
        padded[:new_h, :new_w] = resized
        
        # Convert to RGB and normalize
        padded = cv2.cvtColor(padded, cv2.COLOR_BGR2RGB)
        padded = padded.astype(np.float32) / 255.0
        
        # Transpose to CHW format
        padded = np.transpose(padded, (2, 0, 1))
        
        # Add batch dimension
        padded = np.expand_dims(padded, axis=0)
        
        return padded, scale
    
    def postprocess_output(self, output, scale, conf_threshold=0.25):
        """Post-process YOLO output to get detections."""
        # Output shape: (1, num_boxes, 6) where 6 = [x1, y1, x2, y2, conf, class]
        # Coordinates are in corner format (top-left and bottom-right)
        
        detections = []
        
        if len(output.shape) == 3:
            boxes = output[0]  # Remove batch dimension
        else:
            boxes = output
        
        for box in boxes:
            if len(box) < 6:
                continue
            
            x1, y1, x2, y2, conf, cls = box[:6]
            
            if conf < conf_threshold:
                continue
            
            # Coordinates are already in corner format (x1, y1, x2, y2)
            # Scale back to original image coordinates
            x1 = x1 / scale
            y1 = y1 / scale
            x2 = x2 / scale
            y2 = y2 / scale
            
            cls_id = int(cls)
            cls_name = D4LA_CLASS_NAMES.get(cls_id, "unknown")
            
            detections.append({
                "bbox": [x1, y1, x2, y2],
                "class_id": cls_id,
                "class_name": cls_name,
                "confidence": float(conf)
            })
        
        return detections
    
    def detect(self, image_path, conf_threshold=0.25):
        """Detect layout elements in image."""
        if not self.load_model():
            return []
        
        try:
            # Preprocess
            input_data, scale = self.preprocess_image(image_path)
            
            # Run inference
            input_name = self.session.get_inputs()[0].name
            output = self.session.run(None, {input_name: input_data})[0]
            
            # Postprocess
            detections = self.postprocess_output(output, scale, conf_threshold)
            
            return detections
        except Exception as e:
            print(f"Detection error: {e}", file=sys.stderr)
            return []


class TranslationCache:
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
            except:
                pass
    
    def get(self, text):
        key = hashlib.md5(text.encode()).hexdigest()
        return self.cache.get(key)
    
    def set(self, text, translation):
        key = hashlib.md5(text.encode()).hexdigest()
        self.cache[key] = translation


def contains_math(text):
    """Check if text is primarily mathematical content."""
    # Count math-related characters
    math_chars = 0
    total_chars = len(text)
    
    if total_chars == 0:
        return False
    
    # Greek letters
    greek = "αβγδεζηθικλμνξοπρστυφχψωΑΒΓΔΕΖΗΘΙΚΛΜΝΞΟΠΡΣΤΥΦΧΨΩ"
    # Math symbols
    math_symbols = "∫∑∏√∂∇±×÷≤≥≠≈∞∈∉⊂⊃∪∩∧∨¬∀∃∝∆∇⊕⊗"
    
    for c in text:
        if c in greek or c in math_symbols:
            math_chars += 1
    
    # If more than 5% math symbols, likely a formula
    if math_chars > 0 and math_chars / total_chars > 0.05:
        return True
    
    # Check for LaTeX-style commands (strong indicator)
    if re.search(r'\\[a-zA-Z]{2,}', text):
        return True
    
    # Check for standalone equations: lines that are mostly math
    # e.g., "A+ = (1 + λneg)(1 − p)"
    lines = text.split('\n')
    for line in lines:
        line = line.strip()
        if len(line) < 5:
            continue
        # Count operators and single letters
        operators = sum(1 for c in line if c in '=<>+-*/^_')
        letters = sum(1 for c in line if c.isalpha())
        # Only flag as math if it's a short line with many operators
        if len(line) < 50 and operators > 5 and letters < 15:
            return True
    
    return False


def is_translatable_text(text):
    """Check if text block should be translated."""
    text = text.strip()
    if not text:
        return False
    
    # Too short
    if len(text) < 15:
        return False
    
    # Skip if already has Chinese
    chinese_count = sum(1 for c in text if '\u4e00' <= c <= '\u9fff')
    if chinese_count > len(text) * 0.1:
        return False
    
    # Skip page numbers
    if re.match(r'^\d+$', text):
        return False
    
    # Skip URLs
    if re.match(r'^https?://', text):
        return False
    
    # Skip email addresses
    if re.match(r'^[\w\.\-]+@[\w\.\-]+\.\w+$', text):
        return False
    
    # Skip author names with email (e.g., "John Doe john@example.com")
    if re.search(r'[\w\.\-]+@[\w\.\-]+\.\w+', text) and len(text) < 150:
        return False
    
    # Skip reference patterns [1], [2,3]
    if re.match(r'^\[\d+(?:[-,]\d+)*\]$', text):
        return False
    
    # Skip ONLY short figure/table labels like "Figure 1:" or "Table 2:" (not full sentences)
    if re.match(r'^(Figure|Table|Fig\.|Tab\.)\s*\d+\s*[:.]?\s*$', text, re.IGNORECASE):
        return False
    
    # Skip section headers that are just numbers like "1.", "2.1", "A.1"
    if re.match(r'^[A-Z]?\d+(\.\d+)*\.?\s*$', text):
        return False
    
    # Check alpha ratio - must have substantial text
    alpha_count = sum(1 for c in text if c.isalpha())
    if len(text) > 0 and alpha_count / len(text) < 0.4:
        return False
    
    # Skip if primarily math content
    if contains_math(text):
        return False
    
    return True


def should_translate(text):
    """Check if text should be translated - use relaxed filtering."""
    return is_translatable_text(text)


def fullwidth_to_halfwidth(text):
    """Convert ALL fullwidth characters to halfwidth, including punctuation."""
    if not text:
        return text
    
    result = []
    for char in text:
        code = ord(char)
        
        # Fullwidth ASCII variants (FF01-FF5E) -> ASCII (0021-007E)
        if 0xFF01 <= code <= 0xFF5E:
            result.append(chr(code - 0xFEE0))
        # Fullwidth space -> normal space
        elif code == 0x3000:
            result.append(' ')
        # Chinese/Japanese punctuation -> ASCII
        elif code == 0x3001:  # Ideographic comma
            result.append(',')
        elif code == 0x3002:  # Ideographic full stop
            result.append('.')
        elif code == 0x300c or code == 0x300d:  # Corner brackets
            result.append('"')
        elif code == 0x300e or code == 0x300f:  # White corner brackets
            result.append('"')
        elif code == 0x3010 or code == 0x3011:  # Black lenticular brackets
            result.append('[' if code == 0x3010 else ']')
        elif code == 0x3014 or code == 0x3015:  # Tortoise shell brackets
            result.append('(' if code == 0x3014 else ')')
        elif code == 0x2018 or code == 0x2019:  # Single quotes
            result.append("'")
        elif code == 0x201c or code == 0x201d:  # Double quotes
            result.append('"')
        elif code == 0x2014:  # Em dash
            result.append('-')
        elif code == 0x2013:  # En dash
            result.append('-')
        elif code == 0x2026:  # Ellipsis
            result.append('...')
        elif code == 0x00b7:  # Middle dot
            result.append('.')
        else:
            result.append(char)
    
    return ''.join(result)


def post_process_translation(text):
    """Post-process translation to fix common issues."""
    if not text:
        return text
    # Convert fullwidth to halfwidth
    return fullwidth_to_halfwidth(text)


def has_chinese(text):
    """Check if text contains Chinese characters."""
    if not text:
        return False
    return any('\u4e00' <= c <= '\u9fff' for c in text)


def split_text_into_chunks(text, max_chars=1500):
    """Split long text into smaller chunks at sentence boundaries."""
    if len(text) <= max_chars:
        return [text]
    
    chunks = []
    current_chunk = ""
    
    # Split by sentences (period, question mark, exclamation mark followed by space)
    sentences = re.split(r'(?<=[.!?])\s+', text)
    
    for sentence in sentences:
        if len(current_chunk) + len(sentence) + 1 <= max_chars:
            if current_chunk:
                current_chunk += " " + sentence
            else:
                current_chunk = sentence
        else:
            if current_chunk:
                chunks.append(current_chunk)
            # If single sentence is too long, split by comma or semicolon
            if len(sentence) > max_chars:
                sub_parts = re.split(r'(?<=[,;])\s+', sentence)
                sub_chunk = ""
                for part in sub_parts:
                    if len(sub_chunk) + len(part) + 1 <= max_chars:
                        if sub_chunk:
                            sub_chunk += " " + part
                        else:
                            sub_chunk = part
                    else:
                        if sub_chunk:
                            chunks.append(sub_chunk)
                        sub_chunk = part
                current_chunk = sub_chunk
            else:
                current_chunk = sentence
    
    if current_chunk:
        chunks.append(current_chunk)
    
    return chunks


def translate_text_single(text, api_key, base_url, model, max_retries=3):
    """Translate a single text chunk using OpenAI-compatible API."""
    headers = {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json"
    }
    
    # Use Chinese prompt for better translation quality
    # The previous English prompt caused issues where the model would return
    # the original text unchanged for certain inputs
    prompt = f"""请将以下英文文本翻译成中文。
要求：
1. 使用半角标点符号（如 . , : ; ! ? 等）
2. 保留专有名词和缩写词不翻译
3. 只输出翻译结果，不要添加任何解释

英文原文：{text}"""
    
    data = {
        "model": model,
        "messages": [{"role": "user", "content": prompt}],
        "temperature": 0.1,
        "max_tokens": min(8192, max(512, len(text) * 3))
    }
    
    last_result = None
    for attempt in range(max_retries):
        try:
            resp = requests.post(f"{base_url}/chat/completions", headers=headers, json=data, timeout=180)
            resp.raise_for_status()
            result = resp.json()["choices"][0]["message"]["content"].strip()
            # Post-process to fix fullwidth characters
            result = post_process_translation(result)
            last_result = result
            
            # Validate: check if result contains Chinese
            if not has_chinese(result):
                # For short texts that might be names/terms, accept after retries
                if len(text) < 100:
                    print(f"  Warning: translation has no Chinese (short text, may be name/term)", file=sys.stderr)
                    if attempt < max_retries - 1:
                        time.sleep(0.5)
                        continue
                    # After all retries, return the result anyway (might be intentionally kept as-is)
                    return result
                else:
                    print(f"  Warning: translation has no Chinese, retrying...", file=sys.stderr)
                    if attempt < max_retries - 1:
                        time.sleep(1)
                        continue
                    return None
            
            return result
        except requests.exceptions.Timeout:
            print(f"  Timeout {attempt + 1}, retrying...", file=sys.stderr)
            time.sleep(2)
        except Exception as e:
            print(f"  Error {attempt + 1}: {e}", file=sys.stderr)
            if attempt < max_retries - 1:
                time.sleep(1)
    
    # If we got a result but it had no Chinese, return it for short texts
    if last_result and len(text) < 100:
        return last_result
    return None


def translate_text(text, api_key, base_url, model, max_retries=3):
    """Translate text using OpenAI-compatible API.
    
    For long texts (>1500 chars), splits into chunks and translates separately,
    then combines the results. This avoids API truncation issues.
    """
    # For short texts, translate directly
    if len(text) <= 1500:
        return translate_text_single(text, api_key, base_url, model, max_retries)
    
    # For long texts, split into chunks
    print(f"  Long text ({len(text)} chars), splitting into chunks...", file=sys.stderr)
    chunks = split_text_into_chunks(text, max_chars=1500)
    print(f"  Split into {len(chunks)} chunks", file=sys.stderr)
    
    translated_chunks = []
    for i, chunk in enumerate(chunks):
        print(f"  Translating chunk {i+1}/{len(chunks)}...", file=sys.stderr)
        translation = translate_text_single(chunk, api_key, base_url, model, max_retries)
        if translation:
            translated_chunks.append(translation)
        else:
            # If any chunk fails, return None
            print(f"  Chunk {i+1} translation failed", file=sys.stderr)
            return None
    
    # Combine translated chunks
    return " ".join(translated_chunks)


def extract_text_blocks(page):
    """Extract text blocks from a page with rotation detection."""
    blocks = page.get_text("dict", flags=pymupdf.TEXT_DEHYPHENATE | pymupdf.TEXT_PRESERVE_WHITESPACE)["blocks"]
    
    result = []
    for block in blocks:
        if block.get("type") != 0:
            continue
        
        bbox = block.get("bbox")
        if not bbox:
            continue
        
        # Get font info from first span and detect rotation
        font_size = 10
        font_name = ""
        rotation = 0  # Default: horizontal (0 degrees)
        
        for line in block.get("lines", []):
            # Get text direction from first line
            dir_vec = line.get("dir", (1, 0))
            # dir = (1, 0) means horizontal left-to-right (0 degrees)
            # dir = (0, -1) means vertical bottom-to-top (90 degrees)
            # dir = (-1, 0) means horizontal right-to-left (180 degrees)
            # dir = (0, 1) means vertical top-to-bottom (270 degrees)
            if abs(dir_vec[0]) < 0.1 and dir_vec[1] < -0.5:
                rotation = 90  # Vertical bottom-to-top
            elif abs(dir_vec[0]) < 0.1 and dir_vec[1] > 0.5:
                rotation = 270  # Vertical top-to-bottom
            elif dir_vec[0] < -0.5:
                rotation = 180  # Horizontal right-to-left
            
            for span in line.get("spans", []):
                font_size = span.get("size", 10)
                font_name = span.get("font", "")
                break
            break
        
        lines_text = []
        for line in block.get("lines", []):
            spans_text = []
            prev_span_end = None
            for span in line.get("spans", []):
                span_text = span.get("text", "")
                if not span_text:
                    continue
                
                span_bbox = span.get("bbox", [0, 0, 0, 0])
                if prev_span_end is not None:
                    gap = span_bbox[0] - prev_span_end
                    if gap > 2:
                        spans_text.append(" ")
                
                spans_text.append(span_text)
                prev_span_end = span_bbox[2]
            
            line_text = "".join(spans_text).strip()
            if line_text:
                lines_text.append(line_text)
        
        block_text = " ".join(lines_text)
        block_text = re.sub(r'\s+', ' ', block_text).strip()
        
        if block_text:
            result.append({
                "text": block_text,
                "bbox": bbox,
                "font_size": font_size,
                "font_name": font_name,
                "rotation": rotation
            })
    
    return result


def get_colored_text_blocks(page):
    """Extract text blocks that have non-black color (links, highlights, etc.)."""
    blocks = page.get_text("dict", flags=pymupdf.TEXT_PRESERVE_WHITESPACE)["blocks"]
    
    result = []
    for block in blocks:
        if block.get("type") != 0:
            continue
        
        for line in block.get("lines", []):
            # Get rotation from line direction
            dir_vec = line.get("dir", (1, 0))
            rotation = 0
            if abs(dir_vec[0]) < 0.1 and dir_vec[1] < -0.5:
                rotation = 90
            elif abs(dir_vec[0]) < 0.1 and dir_vec[1] > 0.5:
                rotation = 270
            elif dir_vec[0] < -0.5:
                rotation = 180
            
            for span in line.get("spans", []):
                color = span.get("color", 0)
                text = span.get("text", "").strip()
                bbox = span.get("bbox")
                
                if not text or not bbox:
                    continue
                
                # Check if color is non-black (colored text)
                if color != 0:
                    r = (color >> 16) & 0xFF
                    g = (color >> 8) & 0xFF
                    b = color & 0xFF
                    
                    # Skip if it's very dark (close to black)
                    if r < 30 and g < 30 and b < 30:
                        continue
                    
                    result.append({
                        "text": text,
                        "bbox": bbox,
                        "color": (r, g, b),
                        "rotation": rotation
                    })
    
    return result


def page_to_image(page, dpi=144):
    """Convert PDF page to image and save to temp file."""
    mat = pymupdf.Matrix(dpi / 72, dpi / 72)
    pix = page.get_pixmap(matrix=mat)
    temp_path = tempfile.mktemp(suffix='.png')
    pix.save(temp_path)
    return temp_path, dpi / 72


def extract_text_in_region(page, bbox, scale):
    """Extract text from a specific region of the page with rotation detection."""
    x1, y1, x2, y2 = bbox
    pdf_rect = pymupdf.Rect(x1 / scale, y1 / scale, x2 / scale, y2 / scale)
    
    # Use dict format to get text direction info
    text_dict = page.get_text("dict", clip=pdf_rect, flags=pymupdf.TEXT_DEHYPHENATE)
    
    lines_text = []
    rotation = 0  # Default: horizontal (0 degrees)
    
    for block in text_dict.get("blocks", []):
        if block.get("type") != 0:  # Skip non-text blocks
            continue
        
        for line in block.get("lines", []):
            # Get text direction from first line
            if not lines_text:
                dir_vec = line.get("dir", (1, 0))
                # dir = (1, 0) means horizontal left-to-right (0 degrees)
                # dir = (0, -1) means vertical bottom-to-top (90 degrees)
                # dir = (-1, 0) means horizontal right-to-left (180 degrees)
                # dir = (0, 1) means vertical top-to-bottom (270 degrees)
                if abs(dir_vec[0]) < 0.1 and dir_vec[1] < -0.5:
                    rotation = 90  # Vertical bottom-to-top
                elif abs(dir_vec[0]) < 0.1 and dir_vec[1] > 0.5:
                    rotation = 270  # Vertical top-to-bottom
                elif dir_vec[0] < -0.5:
                    rotation = 180  # Horizontal right-to-left
            
            spans_text = []
            for span in line.get("spans", []):
                span_text = span.get("text", "")
                if span_text:
                    spans_text.append(span_text)
            
            line_text = "".join(spans_text).strip()
            if line_text:
                lines_text.append(line_text)
    
    text = " ".join(lines_text).strip()
    if not text:
        # Fallback to simple text extraction
        text = page.get_text("text", clip=pdf_rect)
        text = re.sub(r'\s+', ' ', text).strip()
    
    return text, pdf_rect, rotation


def translate_pdf(input_pdf, output_pdf, api_key, base_url, model, cache_path):
    """Main translation function with hybrid mode (layout detection + fallback).
    
    Saves PDF after each page is translated for progressive display.
    Outputs PAGE_COMPLETE:<page_num>:<total_pages> after each page.
    
    IMPORTANT: Uses save-and-reopen approach to avoid PyMuPDF document corruption
    when saving multiple times. Each page is processed, saved, then the document
    is reopened for the next page.
    
    Uses two-phase approach per page:
    1. Collect all translations for the page
    2. Add all redaction annotations, apply them, then add all translation text
    This ensures colored text (like blue hyperlinks) is properly removed.
    """
    import shutil
    
    cache = TranslationCache(cache_path)
    detector = LayoutDetector()
    use_layout = detector.available
    
    # Load SimSun font
    load_simsun_font()
    
    # Copy input to output first (we'll modify output in place)
    shutil.copy2(input_pdf, output_pdf)
    
    # Get total pages
    doc = pymupdf.open(input_pdf)
    total_pages = len(doc)
    doc.close()
    
    total_regions = 0
    translated_count = 0
    
    print(f"Processing {total_pages} pages...")
    if use_layout:
        print("Using hybrid mode: layout detection + text block fallback")
    else:
        print("Layout detection not available, using text-based filtering")
    
    for page_num in range(total_pages):
        # Open the current output file for this page
        doc = pymupdf.open(output_pdf)
        page = doc[page_num]
        page_translated = 0
        translated_rects = []  # Track translated regions to avoid duplicates
        skip_rects = []  # Regions to skip (figures, formulas)
        
        # Collect all translations for this page first
        translations_to_apply = []  # List of (rect, translation, rotation) tuples
        
        if use_layout:
            # Step 1: Layout detection
            img_path, scale = page_to_image(page)
            regions = detector.detect(img_path, conf_threshold=0.2)
            
            try:
                os.remove(img_path)
            except:
                pass
            
            # Sort regions by confidence (highest first) to prioritize better detections
            regions = sorted(regions, key=lambda r: r["confidence"], reverse=True)
            
            # Separate regions into translate and skip, with deduplication
            for region in regions:
                x1, y1, x2, y2 = region["bbox"]
                pdf_rect = (x1 / scale, y1 / scale, x2 / scale, y2 / scale)
                
                if region["class_id"] in SKIP_CLASS_IDS:
                    # Special handling for Table regions: some "tables" are actually text boxes
                    # (like Comment boxes in reviewer responses). Check if it contains translatable text.
                    if region["class_id"] == 16:  # Table
                        text, rect, rotation = extract_text_in_region(page, region["bbox"], scale)
                        # If the "table" contains mostly text (not numbers/symbols), translate it
                        if text and len(text) > 20 and should_translate(text):
                            # This is a text box disguised as a table, translate it
                            if any(rects_overlap(pdf_rect, tr, threshold=0.5) for tr in translated_rects):
                                continue
                            
                            total_regions += 1
                            translation = cache.get(text)
                            if not translation:
                                translation = translate_text(text, api_key, base_url, model)
                                if translation:
                                    cache.set(text, translation)
                                    cache.save()
                            
                            if translation:
                                translations_to_apply.append((rect, translation, rotation))
                                translated_rects.append(pdf_rect)
                            continue
                    skip_rects.append(pdf_rect)
                elif region["class_id"] in TRANSLATE_CLASS_IDS and region["confidence"] > 0.25:
                    # Skip if this region significantly overlaps with already processed region
                    # This prevents duplicate translations of the same text
                    if any(rects_overlap(pdf_rect, tr, threshold=0.5) for tr in translated_rects):
                        continue
                    
                    total_regions += 1
                    text, rect, rotation = extract_text_in_region(page, region["bbox"], scale)
                    
                    if not text or len(text) < 10:
                        continue
                    
                    # Use should_translate for filtering
                    if not should_translate(text):
                        continue
                    
                    # Get translation
                    translation = cache.get(text)
                    if not translation:
                        translation = translate_text(text, api_key, base_url, model)
                        if translation:
                            cache.set(text, translation)
                            cache.save()
                    
                    if translation:
                        translations_to_apply.append((rect, translation, rotation))
                        translated_rects.append(pdf_rect)
            
            # Step 2: Fallback - get text blocks not covered by layout detection
            blocks = extract_text_blocks(page)
            
            for block in blocks:
                bbox = block["bbox"]
                block_text = block["text"]
                block_rotation = block.get("rotation", 0)
                
                # Skip if overlaps with already translated region
                if any(rects_overlap(bbox, tr) for tr in translated_rects):
                    continue
                
                # Skip if overlaps with skip regions (figures, formulas)
                if any(rects_overlap(bbox, sr) for sr in skip_rects):
                    continue
                
                if not should_translate(block_text):
                    continue
                
                total_regions += 1
                
                translation = cache.get(block_text)
                if not translation:
                    translation = translate_text(block_text, api_key, base_url, model)
                    if translation:
                        cache.set(block_text, translation)
                        cache.save()
                
                if translation:
                    rect = pymupdf.Rect(bbox)
                    translations_to_apply.append((rect, translation, block_rotation))
                    translated_rects.append(bbox)
            
            # Step 3: Handle colored text (links, highlights) that may still be visible
            # Even if they're in translated regions, we need to redact them separately
            # because the translation overlay might not cover them completely
            colored_blocks = get_colored_text_blocks(page)
            for cb in colored_blocks:
                bbox = cb["bbox"]
                text = cb["text"]
                
                # Skip if in skip regions (figures, formulas)
                if any(rects_overlap(bbox, sr, threshold=0.3) for sr in skip_rects):
                    continue
                
                if not text or len(text) < 2:
                    continue
                
                # Add to redaction list (we'll redact all colored text)
                rect = pymupdf.Rect(bbox)
                
                # Check if we already have a translation for this exact text
                # If not, translate it
                translation = cache.get(text)
                if not translation:
                    # For very short text, just use the original (might be a symbol)
                    if len(text) < 5 and not any(c.isalpha() for c in text):
                        continue
                    translation = translate_text(text, api_key, base_url, model)
                    if translation:
                        cache.set(text, translation)
                        cache.save()
                
                if translation:
                    # Get rotation from colored block
                    cb_rotation = cb.get("rotation", 0)
                    
                    # Check if this rect overlaps with existing translations
                    # If so, just add redaction without new translation
                    overlaps_existing = any(rects_overlap(bbox, tr, threshold=0.5) for tr in translated_rects)
                    if overlaps_existing:
                        # The area is already being translated, but we still need to redact the colored text
                        # Add to a separate list for redaction only (no new translation)
                        translations_to_apply.append((rect, None, cb_rotation))  # None means redact only, no translation
                    else:
                        translations_to_apply.append((rect, translation, cb_rotation))
                        total_regions += 1
        else:
            # Fallback only: use text-based extraction with math filtering
            blocks = extract_text_blocks(page)
            
            for block in blocks:
                block_text = block["text"]
                bbox = block["bbox"]
                block_rotation = block.get("rotation", 0)
                
                total_regions += 1
                
                if not should_translate(block_text):
                    continue
                
                translation = cache.get(block_text)
                if not translation:
                    translation = translate_text(block_text, api_key, base_url, model)
                    if translation:
                        cache.set(block_text, translation)
                        cache.save()
                
                if translation:
                    rect = pymupdf.Rect(bbox)
                    translations_to_apply.append((rect, translation, block_rotation))
        
        # Phase 2: Apply all redactions first, then add translations
        # First, add all redaction annotations for ALL items (including redact-only items)
        for rect, _, _ in translations_to_apply:
            add_redaction_for_rect(page, rect)
        
        # Apply all redactions at once (removes original text including colored text)
        if translations_to_apply:
            try:
                page.apply_redactions()
            except Exception as e:
                print(f"Warning: redaction failed: {e}", file=sys.stderr)
        
        # Then add translation text (skip items with None translation - those are redact-only)
        for rect, translation, rotation in translations_to_apply:
            if translation is not None:  # Skip redact-only items
                if overlay_translation(page, rect, translation, rotation=rotation):
                    translated_count += 1
                    page_translated += 1
        
        print(f"  Page {page_num + 1}: {page_translated} regions translated")
        
        # Save and close document after each page (save-and-reopen approach)
        # This avoids PyMuPDF document corruption when saving multiple times
        try:
            temp_file = output_pdf + ".tmp"
            # Use tobytes() with optimization options to reduce file size:
            # - garbage=4: maximum garbage collection (remove unused objects)
            # - deflate=True: compress streams
            # Note: removed clean=True to preserve image quality
            pdf_bytes = doc.tobytes(garbage=4, deflate=True)
            doc.close()
            
            with open(temp_file, 'wb') as f:
                f.write(pdf_bytes)
            
            # Replace output with temp file
            os.replace(temp_file, output_pdf)
            
            # Output special marker for Go to detect page completion
            print(f"PAGE_COMPLETE:{page_num + 1}:{total_pages}", flush=True)
        except Exception as e:
            try:
                doc.close()
            except:
                pass
            print(f"Warning: failed to save after page {page_num + 1}: {e}", file=sys.stderr)
    
    # Final optimization: subset fonts to reduce file size
    # This removes unused glyphs from embedded fonts
    try:
        print("Optimizing PDF (subsetting fonts)...", file=sys.stderr)
        doc = pymupdf.open(output_pdf)
        doc.subset_fonts()
        # Save to temp file first, then replace (can't save to original directly)
        temp_file = output_pdf + ".optimized.tmp"
        # Note: removed clean=True to preserve image quality
        doc.save(temp_file, garbage=4, deflate=True)
        doc.close()
        os.replace(temp_file, output_pdf)
        print("PDF optimization complete.", file=sys.stderr)
    except Exception as e:
        print(f"Warning: font subsetting failed: {e}", file=sys.stderr)
    
    cache.save()
    
    print(f"\nTotal: {total_regions}, Translated: {translated_count}")
    return True


def rects_overlap(r1, r2, threshold=0.3):
    """Check if two rectangles overlap significantly."""
    x1 = max(r1[0], r2[0])
    y1 = max(r1[1], r2[1])
    x2 = min(r1[2], r2[2])
    y2 = min(r1[3], r2[3])
    
    if x1 >= x2 or y1 >= y2:
        return False
    
    intersection = (x2 - x1) * (y2 - y1)
    area1 = (r1[2] - r1[0]) * (r1[3] - r1[1])
    area2 = (r2[2] - r2[0]) * (r2[3] - r2[1])
    
    if area1 <= 0 or area2 <= 0:
        return False
    
    return intersection / min(area1, area2) > threshold


def overlay_translation(page, rect, translation, rotation=0, use_simsun=True):
    """Add translation text to page with rotation support and dynamic font sizing.
    Returns True if successful."""
    rect = rect & page.rect
    if rect.is_empty or rect.width < 20 or rect.height < 8:
        return False
    
    # Calculate dynamic font size based on rotation and available space
    if rotation == 90 or rotation == 270:
        # For vertical text, use width as the constraint
        available_space = rect.width
    else:
        # For horizontal text, use height as the constraint
        available_space = rect.height
    
    # Dynamic font size: 9-14pt based on available space
    base_font_size = max(9, min(available_space * 0.6, 14))
    
    # Try to find SimSun font file path
    simsun_path = None
    if use_simsun:
        for font_path in [
            'C:/Windows/Fonts/simsun.ttc',
            'C:/Windows/Fonts/simsun.ttf',
            'C:/Windows/Fonts/SimSun.ttc',
            '/usr/share/fonts/truetype/simsun.ttf',
            '/System/Library/Fonts/STHeiti Light.ttc'
        ]:
            if os.path.exists(font_path):
                simsun_path = font_path
                break
    
    # Try textbox with SimSun font file first (best quality)
    if simsun_path:
        for scale in [1.0, 0.95, 0.9, 0.85, 0.8, 0.75, 0.7, 0.65, 0.6, 0.55, 0.5]:
            fs = base_font_size * scale
            if fs < 5.5:
                break
            try:
                rc = page.insert_textbox(
                    rect, translation, 
                    fontsize=fs,
                    fontname="simsun",
                    fontfile=simsun_path,
                    align=pymupdf.TEXT_ALIGN_LEFT,
                    rotate=rotation
                )
                if rc >= 0:
                    return True
            except:
                pass
    
    # Fallback: try with font buffer if available
    if use_simsun and SIMSUN_FONT_BUFFER is not None:
        try:
            page.insert_font(fontname="simsun", fontbuffer=SIMSUN_FONT_BUFFER)
            for scale in [1.0, 0.9, 0.8, 0.7, 0.6]:
                fs = base_font_size * scale
                if fs < 5.5:
                    break
                try:
                    rc = page.insert_textbox(
                        rect, translation, 
                        fontsize=fs,
                        fontname="simsun",
                        align=pymupdf.TEXT_ALIGN_LEFT,
                        rotate=rotation
                    )
                    if rc >= 0:
                        return True
                except:
                    pass
        except:
            pass
    
    # Last resort: use china-s font (built-in Chinese font)
    for scale in [0.8, 0.7, 0.6, 0.5]:
        fs = base_font_size * scale
        if fs < 5.5:
            break
        try:
            rc = page.insert_textbox(
                rect, translation,
                fontsize=fs,
                fontname="china-s",
                align=pymupdf.TEXT_ALIGN_LEFT,
                rotate=rotation
            )
            if rc >= 0:
                return True
        except:
            pass
    
    return False


def add_redaction_for_rect(page, rect, expand=0):
    """Add a redaction annotation for a rectangle.
    
    Args:
        page: PyMuPDF page object
        rect: Rectangle to redact
        expand: Pixels to expand the rectangle (default 0 to avoid covering nearby content like tables)
    """
    rect = rect & page.rect
    if rect.is_empty:
        return
    
    # Expand the rectangle slightly to ensure complete coverage (if expand > 0)
    if expand > 0:
        cover_rect = pymupdf.Rect(
            max(0, rect.x0 - expand),
            max(0, rect.y0 - expand),
            min(page.rect.width, rect.x1 + expand),
            min(page.rect.height, rect.y1 + expand)
        )
    else:
        cover_rect = rect
    
    page.add_redact_annot(cover_rect, fill=(1, 1, 1))


if __name__ == '__main__':
    if len(sys.argv) < 3:
        print("Usage: script.py <input_pdf> <output_pdf> [cache_path]", file=sys.stderr)
        sys.exit(1)
    
    input_pdf = sys.argv[1]
    output_pdf = sys.argv[2]
    cache_path = sys.argv[3] if len(sys.argv) > 3 else None
    
    api_key = os.environ.get("OPENAI_API_KEY", "")
    base_url = os.environ.get("OPENAI_BASE_URL", "https://api.openai.com/v1")
    model = os.environ.get("OPENAI_MODEL", "gpt-4")
    
    if not api_key:
        print("Error: OPENAI_API_KEY not set", file=sys.stderr)
        sys.exit(1)
    
    if not os.path.exists(input_pdf):
        print(f"Error: PDF not found: {input_pdf}", file=sys.stderr)
        sys.exit(1)
    
    print(f"Input: {input_pdf}")
    print(f"Output: {output_pdf}")
    print(f"Model: {model}")
    
    success = translate_pdf(input_pdf, output_pdf, api_key, base_url, model, cache_path)
    sys.exit(0 if success else 1)
`


// Pdf2zhTranslateScript uses pdf2zh (PDFMathTranslate) for high-quality translation
const Pdf2zhTranslateScript = `#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""PDF translation using pdf2zh (PDFMathTranslate)."""
import sys
import os

def translate_with_pdf2zh(input_pdf, output_pdf, service, api_key, base_url, model):
    try:
        from pdf2zh import translate
    except ImportError:
        print("Error: pdf2zh not installed", file=sys.stderr)
        return False
    
    if api_key:
        os.environ["OPENAI_API_KEY"] = api_key
    if base_url:
        os.environ["OPENAI_BASE_URL"] = base_url
    if model:
        os.environ["OPENAI_MODEL"] = model
    
    try:
        mono_path, dual_path = translate(
            input_pdf, output=output_pdf, service=service, lang_in="en", lang_out="zh",
        )
        print(f"Mono PDF: {mono_path}")
        print(f"Dual PDF: {dual_path}")
        return True
    except Exception as e:
        print(f"pdf2zh error: {e}", file=sys.stderr)
        return False

if __name__ == '__main__':
    if len(sys.argv) < 3:
        print("Usage: script.py <input_pdf> <output_pdf> [service]", file=sys.stderr)
        sys.exit(1)
    
    input_pdf, output_pdf = sys.argv[1], sys.argv[2]
    service = sys.argv[3] if len(sys.argv) > 3 else "openai"
    
    api_key = os.environ.get("OPENAI_API_KEY", "")
    base_url = os.environ.get("OPENAI_BASE_URL", "")
    model = os.environ.get("OPENAI_MODEL", "")
    
    if not os.path.exists(input_pdf):
        print(f"Error: PDF not found: {input_pdf}", file=sys.stderr)
        sys.exit(1)
    
    success = translate_with_pdf2zh(input_pdf, output_pdf, service, api_key, base_url, model)
    sys.exit(0 if success else 1)
`

// ExtractTextPyScript extracts text from PDF
const ExtractTextPyScript = `#!/usr/bin/env python3
# -*- coding: utf-8 -*-
import sys, json, os, re
try:
    import pymupdf
except ImportError:
    try:
        import fitz as pymupdf
    except ImportError:
        print("Error: PyMuPDF not installed", file=sys.stderr)
        sys.exit(1)

def is_formula(text):
    # Strong math symbols that clearly indicate formulas
    strong_math_symbols = "∫∑∏√∂∇"
    if any(c in text for c in strong_math_symbols):
        return True
    
    # For short text, check for LaTeX commands
    # For long text (>100 chars), only mark as formula if it's mostly math
    latex_matches = re.findall(r'\\[a-zA-Z]+', text)
    if latex_matches:
        # If text is short, any LaTeX command makes it a formula
        if len(text) < 100:
            return True
        # For long text, only if LaTeX commands are frequent
        latex_ratio = len(''.join(latex_matches)) / len(text)
        if latex_ratio > 0.1:  # More than 10% is LaTeX
            return True
    
    # Equation number pattern like (5) or (A.1)
    if re.search(r'^\s*\(\d+\)\s*$|^\s*\([A-Z]\.\d+\)\s*$', text.strip()):
        return True
    
    # Subscript/superscript patterns (only for short text)
    if len(text) < 50:
        subscript_count = len(re.findall(r'[a-zA-Z][_^]\d', text))
        if subscript_count > 2:
            return True
    
    # Count math-related characters
    math_symbols = "±×÷≤≥≠≈∞∈∉⊂⊃∪∩∧∨¬∀∃αβγδεζηθικλμνξοπρστυφχψωΓΔΘΛΞΠΣΦΨΩ"
    math_count = sum(1 for c in text if c in math_symbols)
    if len(text) > 0 and math_count / len(text) > 0.4:
        return True
    
    return False

def should_translate(text):
    text = text.strip()
    if not text or len(text) < 3:
        return False
    if is_formula(text):
        return False
    alpha_count = sum(1 for c in text if c.isalpha())
    if len(text) > 0 and alpha_count / len(text) < 0.3:
        return False
    chinese_count = sum(1 for c in text if '\u4e00' <= c <= '\u9fff')
    if chinese_count > len(text) * 0.3:
        return False
    return True

def extract_text_blocks(pdf_path, output_json):
    doc = pymupdf.open(pdf_path)
    blocks_data = []
    block_id = 0
    page_count = len(doc)
    
    for page_num in range(page_count):
        page = doc[page_num]
        # Use dict format to get text direction info
        text_dict = page.get_text("dict", flags=pymupdf.TEXT_DEHYPHENATE)
        
        for block in text_dict.get("blocks", []):
            if block.get("type") != 0:  # Skip non-text blocks
                continue
            
            bbox = block.get("bbox")
            if not bbox:
                continue
            
            # Extract text and detect rotation
            lines_text = []
            rotation = 0  # Default: horizontal (0 degrees)
            
            for line in block.get("lines", []):
                # Get text direction from first line
                if not lines_text:
                    dir_vec = line.get("dir", (1, 0))
                    # dir = (1, 0) means horizontal left-to-right (0 degrees)
                    # dir = (0, -1) means vertical bottom-to-top (90 degrees)
                    # dir = (-1, 0) means horizontal right-to-left (180 degrees)
                    # dir = (0, 1) means vertical top-to-bottom (270 degrees)
                    if abs(dir_vec[0]) < 0.1 and dir_vec[1] < -0.5:
                        rotation = 90  # Vertical bottom-to-top
                    elif abs(dir_vec[0]) < 0.1 and dir_vec[1] > 0.5:
                        rotation = 270  # Vertical top-to-bottom
                    elif dir_vec[0] < -0.5:
                        rotation = 180  # Horizontal right-to-left
                
                spans_text = []
                for span in line.get("spans", []):
                    span_text = span.get("text", "")
                    if span_text:
                        spans_text.append(span_text)
                
                line_text = "".join(spans_text).strip()
                if line_text:
                    lines_text.append(line_text)
            
            text = " ".join(lines_text).strip()
            if not text:
                continue
            
            block_type = "formula" if is_formula(text) else "text"
            translatable = should_translate(text)
            
            blocks_data.append({
                "id": f"page_{page_num}_block_{block_id}",
                "page": page_num,
                "x": bbox[0], "y": bbox[1],
                "width": bbox[2] - bbox[0], "height": bbox[3] - bbox[1],
                "text": text,
                "block_type": block_type,
                "translatable": translatable,
                "rotation": rotation
            })
            block_id += 1
    
    doc.close()
    with open(output_json, 'w', encoding='utf-8') as f:
        json.dump(blocks_data, f, ensure_ascii=False, indent=2)
    print(f"Extracted {len(blocks_data)} blocks")
    return True

if __name__ == '__main__':
    if len(sys.argv) < 3:
        print("Usage: script.py <input_pdf> <output_json>", file=sys.stderr)
        sys.exit(1)
    success = extract_text_blocks(sys.argv[1], sys.argv[2])
    sys.exit(0 if success else 1)
`

// WriteTranslationsPyScript writes translations to PDF using redaction method with SimSun font
const WriteTranslationsPyScript = `#!/usr/bin/env python3
# -*- coding: utf-8 -*-
import sys, json, os, re
try:
    import pymupdf
except ImportError:
    try:
        import fitz as pymupdf
    except ImportError:
        print("Error: PyMuPDF not installed", file=sys.stderr)
        sys.exit(1)

WHITE = pymupdf.pdfcolor["white"]

def fullwidth_to_halfwidth(text):
    """Convert fullwidth characters to halfwidth for math expressions"""
    result = []
    for c in text:
        code = ord(c)
        # Fullwidth ASCII variants (FF01-FF5E) -> ASCII (0021-007E)
        if 0xFF01 <= code <= 0xFF5E:
            result.append(chr(code - 0xFEE0))
        # Fullwidth space
        elif code == 0x3000:
            result.append(' ')
        else:
            result.append(c)
    return ''.join(result)

def clean_translation(text):
    """Remove batch separator and clean up translation text"""
    # Remove batch separator and everything after it
    if '---BLOCK_SEPARATOR---' in text:
        text = text.split('---BLOCK_SEPARATOR---')[0]
    
    # Convert fullwidth to halfwidth for math expressions
    text = fullwidth_to_halfwidth(text)
    
    # Remove trailing English text (original text that was appended)
    # Split by double newline and take only Chinese parts
    lines = text.strip().split('\n')
    cleaned_lines = []
    
    for line in lines:
        line = line.strip()
        if not line:
            continue
        # Check if line contains Chinese characters
        has_chinese = any('\u4e00' <= c <= '\u9fff' for c in line)
        if has_chinese:
            cleaned_lines.append(line)
        elif not cleaned_lines:
            # If no Chinese yet, keep the line (might be title/name)
            cleaned_lines.append(line)
    
    return '\n'.join(cleaned_lines).strip()

def write_translations(input_pdf, translations_json, output_pdf):
    with open(translations_json, 'r', encoding='utf-8') as f:
        blocks = json.load(f)
    
    # Group blocks by page
    blocks_by_page = {}
    for block in blocks:
        translated_text = clean_translation(block.get("translated_text", ""))
        if not translated_text:
            continue
        page_num = block.get("page", 0)
        if page_num not in blocks_by_page:
            blocks_by_page[page_num] = []
        blocks_by_page[page_num].append((block, translated_text))
    
    print(f'Found {sum(len(v) for v in blocks_by_page.values())} blocks to translate across {len(blocks_by_page)} pages')
    
    doc = pymupdf.open(input_pdf)
    translated_count = 0
    failed_count = 0
    simsun_count = 0
    htmlbox_count = 0
    rotation_stats = {}
    
    # Try to find SimSun font
    simsun_path = None
    for font_path in [
        'C:/Windows/Fonts/simsun.ttc',
        'C:/Windows/Fonts/simsun.ttf',
        'C:/Windows/Fonts/SimSun.ttc',
        '/usr/share/fonts/truetype/simsun.ttf',
        '/System/Library/Fonts/STHeiti Light.ttc'
    ]:
        if os.path.exists(font_path):
            simsun_path = font_path
            print(f'Found SimSun font: {font_path}')
            break
    
    if not simsun_path:
        print('Warning: SimSun font not found, using default Chinese font')
    
    for page_num in sorted(blocks_by_page.keys()):
        if page_num >= len(doc):
            continue
        
        print(f'Processing page {page_num + 1}...')
        page = doc[page_num]
        page_blocks = blocks_by_page[page_num]
        
        # Step 1: Add all redaction annotations (no expansion to avoid covering tables)
        for block, translated_text in page_blocks:
            rect = pymupdf.Rect(block["x"], block["y"], 
                               block["x"] + block["width"], block["y"] + block["height"])
            rect = rect & page.rect
            if not rect.is_empty:
                # Don't expand rect to avoid covering nearby content like tables
                page.add_redact_annot(rect, fill=WHITE)
        
        # Step 2: Apply all redactions
        page.apply_redactions()
        
        # Step 3: Add translated text with SimSun font and rotation support
        for block, translated_text in page_blocks:
            rect = pymupdf.Rect(block["x"], block["y"], 
                               block["x"] + block["width"], block["y"] + block["height"])
            rect = rect & page.rect
            if rect.is_empty:
                continue
            
            # Get rotation from block data (default 0 = horizontal)
            rotation = block.get("rotation", 0)
            rotation_stats[rotation] = rotation_stats.get(rotation, 0) + 1
            
            # Calculate font size based on rotation
            if rotation == 90 or rotation == 270:
                # For vertical text, use width as the constraint
                available_space = rect.width
                font_size = max(9, min(available_space * 0.6, 14))
            else:
                # For horizontal text, use height as the constraint
                available_space = rect.height
                font_size = max(9, min(available_space * 0.6, 14))
            
            success = False
            
            # Try textbox with SimSun font first
            if simsun_path:
                try:
                    # Try progressively smaller fonts
                    for scale in [1.0, 0.95, 0.9, 0.85, 0.8, 0.75, 0.7, 0.65]:
                        smaller_font = font_size * scale
                        if smaller_font < 7:
                            break
                        rc = page.insert_textbox(
                            rect, translated_text, 
                            fontsize=smaller_font,
                            fontname="simsun",
                            fontfile=simsun_path,
                            align=pymupdf.TEXT_ALIGN_LEFT,
                            rotate=rotation
                        )
                        if rc >= 0:
                            success = True
                            simsun_count += 1
                            break
                except Exception as e:
                    pass
            
            # If SimSun textbox failed, try with even smaller font
            if not success and simsun_path:
                try:
                    for scale in [0.6, 0.55, 0.5, 0.45, 0.4]:
                        smaller_font = font_size * scale
                        if smaller_font < 5:
                            break
                        rc = page.insert_textbox(
                            rect, translated_text, 
                            fontsize=smaller_font,
                            fontname="simsun",
                            fontfile=simsun_path,
                            align=pymupdf.TEXT_ALIGN_LEFT,
                            rotate=rotation
                        )
                        if rc >= 0:
                            success = True
                            simsun_count += 1
                            break
                except:
                    pass
            
            # Last resort: use china-s font (built-in Chinese font)
            if not success:
                try:
                    rc = page.insert_textbox(
                        rect, translated_text,
                        fontsize=font_size * 0.75,
                        fontname="china-s",
                        align=pymupdf.TEXT_ALIGN_LEFT,
                        rotate=rotation
                    )
                    if rc >= 0:
                        success = True
                except:
                    pass
            
            if success:
                translated_count += 1
            else:
                failed_count += 1
    
    print(f'\nTranslated: {translated_count}, Failed: {failed_count}')
    print(f'SimSun textbox: {simsun_count}')
    print(f'Rotation stats: {rotation_stats}')
    print(f'Saving to {output_pdf}...')
    
    try:
        doc.subset_fonts()
    except:
        pass
    
    doc.ez_save(output_pdf)
    doc.close()
    
    print(f'\nSuccess! SimSun translated PDF saved')
    return True

if __name__ == '__main__':
    if len(sys.argv) < 4:
        print("Usage: script.py <input_pdf> <translations_json> <output_pdf>", file=sys.stderr)
        sys.exit(1)
    success = write_translations(sys.argv[1], sys.argv[2], sys.argv[3])
    sys.exit(0 if success else 1)
`
