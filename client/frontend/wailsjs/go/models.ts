export namespace config {
	
	export class ClientConfig {
	    server_url: string;
	    api_key: string;
	    client_id: string;
	    download_path: string;
	    site_mappings?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new ClientConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.server_url = source["server_url"];
	        this.api_key = source["api_key"];
	        this.client_id = source["client_id"];
	        this.download_path = source["download_path"];
	        this.site_mappings = source["site_mappings"];
	    }
	}

}

export namespace main {
	
	export class APIKeyDTO {
	    key: string;
	    name: string;
	    created_at: string;
	
	    static createFrom(source: any = {}) {
	        return new APIKeyDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.name = source["name"];
	        this.created_at = source["created_at"];
	    }
	}
	export class ClientDTO {
	    id: string;
	    last_seen: string;
	    ip: string;
	
	    static createFrom(source: any = {}) {
	        return new ClientDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.last_seen = source["last_seen"];
	        this.ip = source["ip"];
	    }
	}
	export class ServerConfigDTO {
	    api_key: string;
	    storage_path: string;
	    max_concurrent_downloads: number;
	
	    static createFrom(source: any = {}) {
	        return new ServerConfigDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.api_key = source["api_key"];
	        this.storage_path = source["storage_path"];
	        this.max_concurrent_downloads = source["max_concurrent_downloads"];
	    }
	}
	export class ServerFileInfo {
	    id: string;
	    name: string;
	    size: number;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new ServerFileInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.size = source["size"];
	        this.status = source["status"];
	    }
	}

}

export namespace sync {
	
	export class SyncJob {
	    id: string;
	    url: string;
	    file_name: string;
	    total_size: number;
	    status: string;
	    root_hash: string;
	
	    static createFrom(source: any = {}) {
	        return new SyncJob(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.url = source["url"];
	        this.file_name = source["file_name"];
	        this.total_size = source["total_size"];
	        this.status = source["status"];
	        this.root_hash = source["root_hash"];
	    }
	}

}

