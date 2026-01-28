#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
PDF Text Overlay Script
使用 PyMuPDF (fitz) 精确覆盖 PDF 中的文本
"""

import sys
import json
import fitz  # PyMuPDF


def overlay_text_on_pdf(input_pdf, blocks_json, output_pdf):
    """
    在 PDF 上覆盖翻译文本
    
    Args:
        input_pdf: 输入 PDF 路径
        blocks_json: JSON 格式的文本块数据
        output_pdf: 输出 PDF 路径
    """
    try:
        # 解析 JSON 数据
        blocks = json.loads(blocks_json)
        
        # 打开 PDF
        doc = fitz.open(input_pdf)
        
        print(f"Original PDF has {len(doc)} pages")
        
        # 按页面分组文本块
        page_blocks = {}
        for block in blocks:
            page_num = block['page']
            if page_num not in page_blocks:
                page_blocks[page_num] = []
            page_blocks[page_num].append(block)
        
        print(f"Translation blocks cover {len(page_blocks)} pages")
        
        # 处理每一页 - 重要：遍历所有页面，而不仅仅是有翻译块的页面
        for page_index in range(len(doc)):
            page_num = page_index + 1  # 我们的页码从 1 开始
            page = doc[page_index]
            page_height = page.rect.height
            
            # 检查这一页是否有翻译块
            blocks_on_page = page_blocks.get(page_num, [])
            
            if not blocks_on_page:
                # 没有翻译块的页面，保持原样
                print(f"Page {page_num}: No translation blocks, keeping original")
                continue
            
            print(f"Page {page_num}: Processing {len(blocks_on_page)} translation blocks")
            
            # 处理该页的每个文本块
            for block in blocks_on_page:
                try:
                    # 提取坐标和尺寸
                    x = float(block['x'])
                    y = float(block['y'])
                    width = float(block['width'])
                    height = float(block['height'])
                    font_size = float(block.get('font_size', 10))
                    translated_text = block['translated_text']
                    
                    if not translated_text:
                        continue
                    
                    # PDF 坐标系统：原点在左下角，Y 轴向上
                    # PyMuPDF 坐标系统：原点在左上角，Y 轴向下
                    # 需要转换坐标
                    y_top = page_height - y - height
                    
                    # 创建矩形区域（左上角坐标，右下角坐标）
                    rect = fitz.Rect(x, y_top, x + width, y_top + height)
                    
                    # 1. 先画白色矩形覆盖原文
                    # 扩大一点矩形确保完全覆盖
                    cover_rect = fitz.Rect(
                        rect.x0 - 2,
                        rect.y0 - 2,
                        rect.x1 + 2,
                        rect.y1 + 2
                    )
                    page.draw_rect(cover_rect, color=None, fill=(1, 1, 1), overlay=True)
                    
                    # 2. 调整字体大小以适应中文
                    # 中文字符通常比英文宽，需要适当缩小
                    adjusted_font_size = adjust_font_size_for_chinese(
                        translated_text, font_size, width, height
                    )
                    
                    # 确保字体大小在合理范围内
                    adjusted_font_size = max(5, min(adjusted_font_size, 14))
                    
                    # 3. 在白色矩形上添加中文文本
                    # 使用内置的中文字体（如果系统有的话）
                    try:
                        # 尝试使用系统中文字体
                        fontname = "china-ss"  # PyMuPDF 内置的简体中文字体
                        
                        # 插入文本，使用 textbox 方法自动换行
                        rc = page.insert_textbox(
                            rect,
                            translated_text,
                            fontsize=adjusted_font_size,
                            fontname=fontname,
                            fontfile=None,
                            color=(0, 0, 0),  # 黑色文字
                            align=fitz.TEXT_ALIGN_LEFT,
                            overlay=True
                        )
                        
                        # 如果文本溢出（rc < 0），尝试更小的字体
                        if rc < 0:
                            smaller_size = adjusted_font_size * 0.8
                            if smaller_size >= 5:
                                page.insert_textbox(
                                    rect,
                                    translated_text,
                                    fontsize=smaller_size,
                                    fontname=fontname,
                                    fontfile=None,
                                    color=(0, 0, 0),
                                    align=fitz.TEXT_ALIGN_LEFT,
                                    overlay=True
                                )
                    
                    except Exception as font_error:
                        # 如果中文字体失败，尝试使用默认字体
                        print(f"Warning: Chinese font failed, using default: {font_error}")
                        page.insert_textbox(
                            rect,
                            translated_text,
                            fontsize=adjusted_font_size,
                            fontname="helv",  # Helvetica 作为后备
                            color=(0, 0, 0),
                            align=fitz.TEXT_ALIGN_LEFT,
                            overlay=True
                        )
                
                except Exception as block_error:
                    print(f"Error processing block on page {page_num}: {block_error}")
                    continue
        
        # 保存修改后的 PDF
        doc.save(output_pdf, garbage=4, deflate=True, clean=True)
        doc.close()
        
        print(f"Successfully created overlay PDF with {len(doc)} pages: {output_pdf}")
        return True
    
    except Exception as e:
        print(f"Error in overlay_text_on_pdf: {e}", file=sys.stderr)
        import traceback
        traceback.print_exc()
        return False


def adjust_font_size_for_chinese(text, original_size, max_width, max_height):
    """
    根据中文文本调整字体大小
    
    Args:
        text: 文本内容
        original_size: 原始字体大小
        max_width: 最大宽度
        max_height: 最大高度
    
    Returns:
        调整后的字体大小
    """
    if max_width <= 0 or max_height <= 0:
        return original_size
    
    # 统计中文和英文字符数量
    chinese_count = 0
    latin_count = 0
    
    for char in text:
        if '\u4e00' <= char <= '\u9fff':  # 中文字符范围
            chinese_count += 1
        elif char.isalpha():
            latin_count += 1
    
    total_chars = chinese_count + latin_count
    if total_chars == 0:
        return original_size
    
    # 估算文本宽度
    # 中文字符宽度约等于字体大小
    # 英文字符宽度约为字体大小的 0.5 倍
    estimated_width = chinese_count * original_size + latin_count * original_size * 0.5
    
    # 如果文本宽度超出，按比例缩小
    if estimated_width > max_width:
        scale_factor = max_width / estimated_width
        adjusted_size = original_size * scale_factor
        
        # 考虑多行的情况
        # 如果缩小后的字体太小，可能需要多行显示
        if adjusted_size < 6:
            # 计算需要多少行
            lines_needed = estimated_width / max_width
            line_height = original_size * 1.2
            
            # 检查是否有足够的高度容纳多行
            if lines_needed * line_height <= max_height:
                # 可以多行显示，使用稍大的字体
                adjusted_size = min(original_size, max_height / (lines_needed * 1.2))
            else:
                # 高度不够，只能用小字体
                adjusted_size = 6
        
        return adjusted_size
    
    return original_size


def main():
    """主函数"""
    if len(sys.argv) != 4:
        print("Usage: python overlay_pdf.py <input_pdf> <blocks_json> <output_pdf>")
        sys.exit(1)
    
    input_pdf = sys.argv[1]
    blocks_json = sys.argv[2]
    output_pdf = sys.argv[3]
    
    success = overlay_text_on_pdf(input_pdf, blocks_json, output_pdf)
    
    if not success:
        sys.exit(1)


if __name__ == "__main__":
    main()
