export namespace compiler {
	
	export class LaTeXCompiler {
	
	
	    static createFrom(source: any = {}) {
	        return new LaTeXCompiler(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}

}

export namespace config {
	
	export class ConfigManager {
	
	
	    static createFrom(source: any = {}) {
	        return new ConfigManager(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}

}

export namespace downloader {
	
	export class SourceDownloader {
	
	
	    static createFrom(source: any = {}) {
	        return new SourceDownloader(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}

}

export namespace errors {
	
	export class ErrorRecord {
	    id: string;
	    title: string;
	    input: string;
	    stage: string;
	    error_msg: string;
	    // Go type: time
	    timestamp: any;
	    can_retry: boolean;
	    retry_count: number;
	    // Go type: time
	    last_retry: any;
	
	    static createFrom(source: any = {}) {
	        return new ErrorRecord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.input = source["input"];
	        this.stage = source["stage"];
	        this.error_msg = source["error_msg"];
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.can_retry = source["can_retry"];
	        this.retry_count = source["retry_count"];
	        this.last_retry = this.convertValues(source["last_retry"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace github {
	
	export class ArxivPaperInfo {
	    arxiv_id: string;
	    chinese_pdf: string;
	    bilingual_pdf: string;
	    latex_zip: string;
	    chinese_pdf_url: string;
	    bilingual_pdf_url: string;
	    latex_zip_url: string;
	    has_chinese: boolean;
	    has_bilingual: boolean;
	    has_latex: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ArxivPaperInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.arxiv_id = source["arxiv_id"];
	        this.chinese_pdf = source["chinese_pdf"];
	        this.bilingual_pdf = source["bilingual_pdf"];
	        this.latex_zip = source["latex_zip"];
	        this.chinese_pdf_url = source["chinese_pdf_url"];
	        this.bilingual_pdf_url = source["bilingual_pdf_url"];
	        this.latex_zip_url = source["latex_zip_url"];
	        this.has_chinese = source["has_chinese"];
	        this.has_bilingual = source["has_bilingual"];
	        this.has_latex = source["has_latex"];
	    }
	}
	export class TranslationSearchResult {
	    found: boolean;
	    arxiv_id: string;
	    chinese_pdf: string;
	    bilingual_pdf: string;
	    latex_zip: string;
	    download_url_cn: string;
	    download_url_bi: string;
	    download_url_zip: string;
	    chinese_pdf_filename: string;
	    bilingual_pdf_filename: string;
	    latex_zip_filename: string;
	
	    static createFrom(source: any = {}) {
	        return new TranslationSearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.found = source["found"];
	        this.arxiv_id = source["arxiv_id"];
	        this.chinese_pdf = source["chinese_pdf"];
	        this.bilingual_pdf = source["bilingual_pdf"];
	        this.latex_zip = source["latex_zip"];
	        this.download_url_cn = source["download_url_cn"];
	        this.download_url_bi = source["download_url_bi"];
	        this.download_url_zip = source["download_url_zip"];
	        this.chinese_pdf_filename = source["chinese_pdf_filename"];
	        this.bilingual_pdf_filename = source["bilingual_pdf_filename"];
	        this.latex_zip_filename = source["latex_zip_filename"];
	    }
	}

}

export namespace main {
	
	export class ArxivPaperMetadata {
	    arxiv_id: string;
	    title: string;
	    abstract: string;
	    authors: string;
	
	    static createFrom(source: any = {}) {
	        return new ArxivPaperMetadata(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.arxiv_id = source["arxiv_id"];
	        this.title = source["title"];
	        this.abstract = source["abstract"];
	        this.authors = source["authors"];
	    }
	}
	export class PaperListItem {
	    arxiv_id: string;
	    title: string;
	    translated_at: string;
	
	    static createFrom(source: any = {}) {
	        return new PaperListItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.arxiv_id = source["arxiv_id"];
	        this.title = source["title"];
	        this.translated_at = source["translated_at"];
	    }
	}
	export class ShareCheckResult {
	    can_share: boolean;
	    chinese_pdf_exists: boolean;
	    bilingual_pdf_exists: boolean;
	    chinese_pdf_path: string;
	    bilingual_pdf_path: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new ShareCheckResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.can_share = source["can_share"];
	        this.chinese_pdf_exists = source["chinese_pdf_exists"];
	        this.bilingual_pdf_exists = source["bilingual_pdf_exists"];
	        this.chinese_pdf_path = source["chinese_pdf_path"];
	        this.bilingual_pdf_path = source["bilingual_pdf_path"];
	        this.message = source["message"];
	    }
	}
	export class ShareResult {
	    success: boolean;
	    chinese_pdf_url?: string;
	    bilingual_pdf_url?: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new ShareResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.chinese_pdf_url = source["chinese_pdf_url"];
	        this.bilingual_pdf_url = source["bilingual_pdf_url"];
	        this.message = source["message"];
	    }
	}
	export class StartupCheckResult {
	    latex_installed: boolean;
	    latex_version: string;
	    llm_configured: boolean;
	    llm_error: string;
	
	    static createFrom(source: any = {}) {
	        return new StartupCheckResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.latex_installed = source["latex_installed"];
	        this.latex_version = source["latex_version"];
	        this.llm_configured = source["llm_configured"];
	        this.llm_error = source["llm_error"];
	    }
	}

}

export namespace pdf {
	
	export class PDFInfo {
	    file_path: string;
	    file_name: string;
	    page_count: number;
	    file_size: number;
	    is_text_pdf: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PDFInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.file_path = source["file_path"];
	        this.file_name = source["file_name"];
	        this.page_count = source["page_count"];
	        this.file_size = source["file_size"];
	        this.is_text_pdf = source["is_text_pdf"];
	    }
	}
	export class PDFStatus {
	    phase: string;
	    progress: number;
	    message: string;
	    total_blocks: number;
	    completed_blocks: number;
	    cached_blocks: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new PDFStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.phase = source["phase"];
	        this.progress = source["progress"];
	        this.message = source["message"];
	        this.total_blocks = source["total_blocks"];
	        this.completed_blocks = source["completed_blocks"];
	        this.cached_blocks = source["cached_blocks"];
	        this.error = source["error"];
	    }
	}
	export class TranslationResult {
	    original_pdf_path: string;
	    translated_pdf_path: string;
	    total_blocks: number;
	    translated_blocks: number;
	    cached_blocks: number;
	    tokens_used: number;
	
	    static createFrom(source: any = {}) {
	        return new TranslationResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.original_pdf_path = source["original_pdf_path"];
	        this.translated_pdf_path = source["translated_pdf_path"];
	        this.total_blocks = source["total_blocks"];
	        this.translated_blocks = source["translated_blocks"];
	        this.cached_blocks = source["cached_blocks"];
	        this.tokens_used = source["tokens_used"];
	    }
	}

}

export namespace results {
	
	export class PaperInfo {
	    arxiv_id: string;
	    title: string;
	    // Go type: time
	    translated_at: any;
	    original_pdf: string;
	    translated_pdf: string;
	    bilingual_pdf?: string;
	    source_dir: string;
	    has_latex_source: boolean;
	    status: string;
	    error_message?: string;
	    last_phase?: string;
	    original_input?: string;
	    main_tex_file?: string;
	    source_type?: string;
	    source_md5?: string;
	    source_file_name?: string;
	
	    static createFrom(source: any = {}) {
	        return new PaperInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.arxiv_id = source["arxiv_id"];
	        this.title = source["title"];
	        this.translated_at = this.convertValues(source["translated_at"], null);
	        this.original_pdf = source["original_pdf"];
	        this.translated_pdf = source["translated_pdf"];
	        this.bilingual_pdf = source["bilingual_pdf"];
	        this.source_dir = source["source_dir"];
	        this.has_latex_source = source["has_latex_source"];
	        this.status = source["status"];
	        this.error_message = source["error_message"];
	        this.last_phase = source["last_phase"];
	        this.original_input = source["original_input"];
	        this.main_tex_file = source["main_tex_file"];
	        this.source_type = source["source_type"];
	        this.source_md5 = source["source_md5"];
	        this.source_file_name = source["source_file_name"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ExistingTranslationInfo {
	    exists: boolean;
	    paper_info?: PaperInfo;
	    is_complete: boolean;
	    can_continue: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new ExistingTranslationInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exists = source["exists"];
	        this.paper_info = this.convertValues(source["paper_info"], PaperInfo);
	        this.is_complete = source["is_complete"];
	        this.can_continue = source["can_continue"];
	        this.message = source["message"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace translator {
	
	export class TranslationEngine {
	
	
	    static createFrom(source: any = {}) {
	        return new TranslationEngine(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}

}

export namespace types {
	
	export class Config {
	    openai_api_key: string;
	    openai_base_url: string;
	    openai_model: string;
	    context_window: number;
	    default_compiler: string;
	    work_directory: string;
	    last_input: string;
	    concurrency: number;
	    github_token: string;
	    github_owner: string;
	    github_repo: string;
	    library_page_size: number;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.openai_api_key = source["openai_api_key"];
	        this.openai_base_url = source["openai_base_url"];
	        this.openai_model = source["openai_model"];
	        this.context_window = source["context_window"];
	        this.default_compiler = source["default_compiler"];
	        this.work_directory = source["work_directory"];
	        this.last_input = source["last_input"];
	        this.concurrency = source["concurrency"];
	        this.github_token = source["github_token"];
	        this.github_owner = source["github_owner"];
	        this.github_repo = source["github_repo"];
	        this.library_page_size = source["library_page_size"];
	    }
	}
	export class SourceInfo {
	    source_type: string;
	    original_ref: string;
	    extract_dir: string;
	    main_tex_file: string;
	    all_tex_files: string[];
	
	    static createFrom(source: any = {}) {
	        return new SourceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source_type = source["source_type"];
	        this.original_ref = source["original_ref"];
	        this.extract_dir = source["extract_dir"];
	        this.main_tex_file = source["main_tex_file"];
	        this.all_tex_files = source["all_tex_files"];
	    }
	}
	export class ProcessResult {
	    original_pdf_path: string;
	    translated_pdf_path: string;
	    bilingual_pdf_path: string;
	    source_info?: SourceInfo;
	    source_id: string;
	
	    static createFrom(source: any = {}) {
	        return new ProcessResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.original_pdf_path = source["original_pdf_path"];
	        this.translated_pdf_path = source["translated_pdf_path"];
	        this.bilingual_pdf_path = source["bilingual_pdf_path"];
	        this.source_info = this.convertValues(source["source_info"], SourceInfo);
	        this.source_id = source["source_id"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class Status {
	    phase: string;
	    progress: number;
	    message: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.phase = source["phase"];
	        this.progress = source["progress"];
	        this.message = source["message"];
	        this.error = source["error"];
	    }
	}

}

export namespace validator {
	
	export class SyntaxValidator {
	
	
	    static createFrom(source: any = {}) {
	        return new SyntaxValidator(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	
	    }
	}

}

