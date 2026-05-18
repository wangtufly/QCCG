export namespace account {
	
	export class Account {
	    id: string;
	    name: string;
	    email?: string;
	    user_type?: string;
	    plan?: string;
	    auth_mode: string;
	    api_mode: string;
	    tags: string[];
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    last_used?: any;
	    active: boolean;
	    sort_order: number;
	
	    static createFrom(source: any = {}) {
	        return new Account(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.email = source["email"];
	        this.user_type = source["user_type"];
	        this.plan = source["plan"];
	        this.auth_mode = source["auth_mode"];
	        this.api_mode = source["api_mode"];
	        this.tags = source["tags"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.last_used = this.convertValues(source["last_used"], null);
	        this.active = source["active"];
	        this.sort_order = source["sort_order"];
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
	export class OAuthSession {
	    login_id: string;
	    login_url: string;
	
	    static createFrom(source: any = {}) {
	        return new OAuthSession(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.login_id = source["login_id"];
	        this.login_url = source["login_url"];
	    }
	}
	export class QuotaBucket {
	    used: number;
	    total: number;
	    remaining: number;
	    reset_time?: string;
	
	    static createFrom(source: any = {}) {
	        return new QuotaBucket(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.used = source["used"];
	        this.total = source["total"];
	        this.remaining = source["remaining"];
	        this.reset_time = source["reset_time"];
	    }
	}
	export class QuotaInfo {
	    plan: string;
	    user_quota?: QuotaBucket;
	    addon_quota?: QuotaBucket;
	    is_quota_exceeded: boolean;
	
	    static createFrom(source: any = {}) {
	        return new QuotaInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.plan = source["plan"];
	        this.user_quota = this.convertValues(source["user_quota"], QuotaBucket);
	        this.addon_quota = this.convertValues(source["addon_quota"], QuotaBucket);
	        this.is_quota_exceeded = source["is_quota_exceeded"];
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
	export class Settings {
	    port: number;
	    auto_start: boolean;
	    log_level: string;
	    quota_refresh_interval: number;
	    bridge_token?: string;
	    model_mapping?: Record<string, string>;
	    model_mappings?: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.port = source["port"];
	        this.auto_start = source["auto_start"];
	        this.log_level = source["log_level"];
	        this.quota_refresh_interval = source["quota_refresh_interval"];
	        this.bridge_token = source["bridge_token"];
	        this.model_mapping = source["model_mapping"];
	        this.model_mappings = source["model_mappings"];
	    }
	}
	export class Status {
	    running: boolean;
	    port: number;
	    active_account: string;
	    api_mode: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.port = source["port"];
	        this.active_account = source["active_account"];
	        this.api_mode = source["api_mode"];
	    }
	}

}

export namespace logger {
	
	export class Entry {
	    seq: number;
	    // Go type: time
	    time: any;
	    level: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.seq = source["seq"];
	        this.time = this.convertValues(source["time"], null);
	        this.level = source["level"];
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
	export class LogPage {
	    entries: Entry[];
	    last_seq: number;
	
	    static createFrom(source: any = {}) {
	        return new LogPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entries = this.convertValues(source["entries"], Entry);
	        this.last_seq = source["last_seq"];
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

export namespace main {
	
	export class ClientConfig {
	    type: string;
	    name: string;
	    icon: string;
	    config_path: string;
	    base_url: string;
	    env_vars: string;
	    model: string;
	    applied: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ClientConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.name = source["name"];
	        this.icon = source["icon"];
	        this.config_path = source["config_path"];
	        this.base_url = source["base_url"];
	        this.env_vars = source["env_vars"];
	        this.model = source["model"];
	        this.applied = source["applied"];
	        this.error = source["error"];
	    }
	}
	export class ClientConfigFile {
	    path: string;
	    content: string;
	    format: string;
	    existed: boolean;
	    extra_files?: ClientConfigFile[];
	
	    static createFrom(source: any = {}) {
	        return new ClientConfigFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.content = source["content"];
	        this.format = source["format"];
	        this.existed = source["existed"];
	        this.extra_files = this.convertValues(source["extra_files"], ClientConfigFile);
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
	export class QoderModel {
	    key: string;
	    display_name: string;
	    enable: boolean;
	    is_default: boolean;
	    max_input_tokens?: number;
	
	    static createFrom(source: any = {}) {
	        return new QoderModel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.display_name = source["display_name"];
	        this.enable = source["enable"];
	        this.is_default = source["is_default"];
	        this.max_input_tokens = source["max_input_tokens"];
	    }
	}

}

