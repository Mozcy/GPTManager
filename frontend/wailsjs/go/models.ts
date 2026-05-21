export namespace main {
	
	export class UsageWindowInfo {
	    usedPercent: number;
	    limitWindowSeconds: number;
	    resetAfterSeconds: number;
	    resetAt: number;
	
	    static createFrom(source: any = {}) {
	        return new UsageWindowInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.usedPercent = source["usedPercent"];
	        this.limitWindowSeconds = source["limitWindowSeconds"];
	        this.resetAfterSeconds = source["resetAfterSeconds"];
	        this.resetAt = source["resetAt"];
	    }
	}
	export class AccountInfo {
	    id: number;
	    provider: string;
	    subject: string;
	    userId: string;
	    accountId: string;
	    email: string;
	    name: string;
	    workspaceName: string;
	    workspaceStructure: string;
	    workspaceCreatedTime: string;
	    workspaceProcessor: string;
	    workspaceRole: string;
	    workspaceProfilePictureId: string;
	    workspaceProfilePictureUrl: string;
	    workspaceEligibleForAutoReactivation: boolean;
	    subscription: string;
	    subscriptionExpiresAt: string;
	    primaryWindow: UsageWindowInfo;
	    secondaryWindow: UsageWindowInfo;
	    active: boolean;
	    expiresAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new AccountInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.provider = source["provider"];
	        this.subject = source["subject"];
	        this.userId = source["userId"];
	        this.accountId = source["accountId"];
	        this.email = source["email"];
	        this.name = source["name"];
	        this.workspaceName = source["workspaceName"];
	        this.workspaceStructure = source["workspaceStructure"];
	        this.workspaceCreatedTime = source["workspaceCreatedTime"];
	        this.workspaceProcessor = source["workspaceProcessor"];
	        this.workspaceRole = source["workspaceRole"];
	        this.workspaceProfilePictureId = source["workspaceProfilePictureId"];
	        this.workspaceProfilePictureUrl = source["workspaceProfilePictureUrl"];
	        this.workspaceEligibleForAutoReactivation = source["workspaceEligibleForAutoReactivation"];
	        this.subscription = source["subscription"];
	        this.subscriptionExpiresAt = source["subscriptionExpiresAt"];
	        this.primaryWindow = this.convertValues(source["primaryWindow"], UsageWindowInfo);
	        this.secondaryWindow = this.convertValues(source["secondaryWindow"], UsageWindowInfo);
	        this.active = source["active"];
	        this.expiresAt = source["expiresAt"];
	        this.updatedAt = source["updatedAt"];
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
	export class UpstreamConfig {
	    type: string;
	    ip: string;
	    port: string;
	
	    static createFrom(source: any = {}) {
	        return new UpstreamConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.ip = source["ip"];
	        this.port = source["port"];
	    }
	}
	export class UpstreamStatus {
	    connected: boolean;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new UpstreamStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connected = source["connected"];
	        this.message = source["message"];
	    }
	}

}

