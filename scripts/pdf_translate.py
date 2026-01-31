#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
PDF Translation using PyMuPDF - 按行提取以匹配 Go 代码

Go 代码使用 ledongthuc/pdf 的 GetTextByRow() 按行提取文本，
所以这里也使用按行提取来匹配缓存。

用法:
    python pdf_translate.py <input_pdf> <cache_json> <output_pdf>

依赖:
    pip install PyMuPDF
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
        print("错误: 请安装 PyMuPDF: pip install PyMuPDF")
        sys.exit(1)


WHITE = pymupdf.pdfcolor["white"]


def load_cache(path):
    """加载翻译缓存"""
    with open(path, 'r', encoding='utf-8') as f:
        data = json.load(f)
    
    cache = {}
    if isinstance(data, dict) and 'entries' in data:
        for e in data['entries']:
            orig = e.get('original', '')
            trans = e.get('translation', '')
            if orig and trans:
                key = re.sub(r'\s+', '', orig)
                cache[key] = trans
    return cache


def normalize_text(text):
    """标准化文本"""
    return re.sub(r'\s+', '', text)


def find_translation(text, cache, cache_keys):
    """查找翻译 - 激进匹配"""
    normalized = normalize_text(text)
    if not normalized or len(normalized) < 3:
        return None
    
    # 完全匹配
    if normalized in cache:
        return cache[normalized]
    
    # 子串匹配 - 缓存在 PDF 中
    for key in cache_keys:
        if len(key) >= 10 and key in normalized:
            return cache[key]
    
    # 子串匹配 - PDF 在缓存中
    if len(normalized) >= 10:
        for key in cache_keys:
            if normalized in key:
                return cache[key]
    
    # 滑动窗口匹配
    if len(normalized) >= 12:
        for key in cache_keys:
            if len(key) >= 12:
                for i in range(len(normalized) - 11):
                    if normalized[i:i+12] in key:
                        return cache[key]
    
    # 前缀匹配
    for key in cache_keys:
        min_len = min(len(normalized), len(key))
        if min_len >= 10:
            prefix_len = 0
            for i in range(min_len):
                if normalized[i] == key[i]:
                    prefix_len += 1
                else:
                    break
            if prefix_len >= 10:
                return cache[key]
    
    return None


def translate_pdf(input_pdf, cache_path, output_pdf):
    """翻译 PDF - 按行处理"""
    print(f"输入: {input_pdf}")
    print(f"缓存: {cache_path}")
    print(f"输出: {output_pdf}")
    
    cache = load_cache(cache_path)
    print(f"加载了 {len(cache)} 条翻译")
    
    cache_keys = sorted(cache.keys(), key=len, reverse=True)
    
    doc = pymupdf.open(input_pdf)
    print(f"PDF 共 {len(doc)} 页")
    
    # 创建 OCG 图层
    ocg = doc.add_ocg("Chinese", on=True)
    
    translated_count = 0
    
    for page_num in range(len(doc)):
        page = doc[page_num]
        print(f"\n处理第 {page_num + 1} 页...")
        
        # 使用 get_text("dict") 获取详细信息，按行处理
        text_dict = page.get_text("dict")
        
        lines_to_process = []
        
        for block in text_dict.get("blocks", []):
            if block.get("type") != 0:
                continue
            
            for line in block.get("lines", []):
                # 收集行中所有文本
                line_text = ""
                spans_in_line = []
                
                for span in line.get("spans", []):
                    span_text = span.get("text", "")
                    line_text += span_text
                    spans_in_line.append(span)
                
                if not line_text.strip():
                    continue
                
                # 获取行的边界框
                line_bbox = pymupdf.Rect(line["bbox"])
                
                # 查找翻译
                translation = find_translation(line_text, cache, cache_keys)
                
                lines_to_process.append({
                    "bbox": line_bbox,
                    "text": line_text,
                    "translation": translation,
                    "spans": spans_in_line
                })
        
        print(f"  找到 {len(lines_to_process)} 行文本")
        
        # 对有翻译的行添加 redaction
        for item in lines_to_process:
            if item["translation"]:
                page.add_redact_annot(item["bbox"], fill=WHITE)
        
        # 应用 redaction
        page.apply_redactions()
        
        # 写入翻译
        page_translated = 0
        for item in lines_to_process:
            if item["translation"]:
                bbox = item["bbox"]
                translation = item["translation"]
                
                try:
                    # 获取原始字体大小
                    font_size = 9
                    if item["spans"]:
                        font_size = item["spans"][0].get("size", 9)
                        font_size = min(max(font_size * 0.9, 6), 12)  # 限制范围
                    
                    html_text = f'<span style="font-size:{font_size:.1f}pt;">{translation}</span>'
                    page.insert_htmlbox(bbox, html_text, oc=ocg)
                    page_translated += 1
                except Exception as e:
                    try:
                        page.insert_textbox(
                            bbox, translation,
                            fontsize=9, fontname="china-ss", oc=ocg
                        )
                        page_translated += 1
                    except:
                        pass
        
        translated_count += page_translated
        print(f"  翻译了 {page_translated} 行")
    
    print(f"\n总共翻译了 {translated_count} 行")
    
    # 验证页数
    original_page_count = len(doc)
    
    doc.subset_fonts()
    doc.ez_save(output_pdf)
    print(f"已保存到: {output_pdf}")
    doc.close()
    
    # 重新打开验证页数
    doc_check = pymupdf.open(output_pdf)
    output_page_count = len(doc_check)
    doc_check.close()
    
    if original_page_count != output_page_count:
        print(f"警告: 页数不一致! 原始: {original_page_count}, 输出: {output_page_count}")
    else:
        print(f"页数验证通过: {output_page_count} 页")


def main():
    if len(sys.argv) != 4:
        print("用法: python pdf_translate.py <input_pdf> <cache_json> <output_pdf>")
        sys.exit(1)
    
    input_pdf = sys.argv[1]
    cache_path = sys.argv[2]
    output_pdf = sys.argv[3]
    
    if not os.path.exists(input_pdf):
        print(f"错误: 输入文件不存在: {input_pdf}")
        sys.exit(1)
    
    if not os.path.exists(cache_path):
        print(f"错误: 缓存文件不存在: {cache_path}")
        sys.exit(1)
    
    translate_pdf(input_pdf, cache_path, output_pdf)


if __name__ == "__main__":
    main()
