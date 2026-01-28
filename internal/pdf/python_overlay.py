#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
PDF overlay script using reportlab
Overlays Chinese text on original PDF
"""

import sys
import json
from reportlab.pdfgen import canvas
from reportlab.lib.pagesizes import A4
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.ttfonts import TTFont
from PyPDF2 import PdfReader, PdfWriter
import os

def overlay_text_on_pdf(original_pdf, blocks_json, output_pdf):
    """
    Overlay Chinese text on PDF
    
    Args:
        original_pdf: Path to original PDF
        blocks_json: JSON string with translated blocks
        output_pdf: Path to output PDF
    """
    # Parse blocks
    blocks = json.loads(blocks_json)
    
    # Read original PDF
    reader = PdfReader(original_pdf)
    writer = PdfWriter()
    
    # Register Chinese font (using system font)
    # Try to find a Chinese font
    font_paths = [
        "C:/Windows/Fonts/msyh.ttc",  # Microsoft YaHei
        "C:/Windows/Fonts/simsun.ttc",  # SimSun
        "C:/Windows/Fonts/simhei.ttf",  # SimHei
    ]
    
    font_registered = False
    for font_path in font_paths:
        if os.path.exists(font_path):
            try:
                pdfmetrics.registerFont(TTFont('Chinese', font_path))
                font_registered = True
                print(f"Registered font: {font_path}")
                break
            except:
                continue
    
    if not font_registered:
        print("Warning: No Chinese font found, using default")
    
    # Group blocks by page
    page_blocks = {}
    for block in blocks:
        page = block.get('page', 1)
        if page not in page_blocks:
            page_blocks[page] = []
        page_blocks[page].append(block)
    
    # Process each page
    for page_num in range(len(reader.pages)):
        page = reader.pages[page_num]
        
        # Check if this page has translations
        if (page_num + 1) in page_blocks:
            # Create overlay
            overlay_path = f"temp_overlay_{page_num}.pdf"
            c = canvas.Canvas(overlay_path, pagesize=A4)
            
            # Add translated text
            for block in page_blocks[page_num + 1]:
                text = block.get('translated_text', '')
                if not text:
                    continue
                
                x = block.get('x', 0)
                y = block.get('y', 0)
                width = block.get('width', 100)
                height = block.get('height', 20)
                font_size = block.get('font_size', 10)
                
                # Adjust font size for Chinese
                font_size = max(8, min(font_size, 16))
                
                # Set font
                if font_registered:
                    c.setFont('Chinese', font_size)
                else:
                    c.setFont('Helvetica', font_size)
                
                # Draw white rectangle background
                c.setFillColorRGB(1, 1, 1, alpha=0.95)
                c.rect(x, y, width, height, fill=1, stroke=0)
                
                # Draw text
                c.setFillColorRGB(0, 0, 0)  # Black text
                c.drawString(x + 2, y + 2, text)
            
            c.save()
            
            # Merge overlay with original page
            overlay_reader = PdfReader(overlay_path)
            page.merge_page(overlay_reader.pages[0])
            
            # Clean up
            os.remove(overlay_path)
        
        writer.add_page(page)
    
    # Write output
    with open(output_pdf, 'wb') as f:
        writer.write(f)
    
    print(f"Output written to: {output_pdf}")

if __name__ == '__main__':
    if len(sys.argv) != 4:
        print("Usage: python_overlay.py <original_pdf> <blocks_json> <output_pdf>")
        sys.exit(1)
    
    original_pdf = sys.argv[1]
    blocks_json = sys.argv[2]
    output_pdf = sys.argv[3]
    
    overlay_text_on_pdf(original_pdf, blocks_json, output_pdf)
