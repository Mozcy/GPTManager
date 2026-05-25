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
	export class CodexAuthInfo {
	    path: string;
	    accountId: string;
	    email: string;
	    subscription: string;
	    workspaceName: string;
	    updatedAt: string;
	    authMode: string;
	    lastRefresh: string;
	    accessToken: string;
	    idToken: string;
	    refreshToken: string;
	    tokenType: string;
	
	    static createFrom(source: any = {}) {
	        return new CodexAuthInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.accountId = source["accountId"];
	        this.email = source["email"];
	        this.subscription = source["subscription"];
	        this.workspaceName = source["workspaceName"];
	        this.updatedAt = source["updatedAt"];
	        this.authMode = source["authMode"];
	        this.lastRefresh = source["lastRefresh"];
	        this.accessToken = source["accessToken"];
	        this.idToken = source["idToken"];
	        this.refreshToken = source["refreshToken"];
	        this.tokenType = source["tokenType"];
	    }
	}
	export class CodexProcessInfo {
	    pid: number;
	    name: string;
	    commandLine: string;
	    executablePath: string;
	    owner: string;
	    creationDate: string;
	    parentPid: number;
	    parentName: string;
	    parentCommandLine: string;
	    childProcesses: string;
	    status: string;
	    threadCount: number;
	    handleCount: number;
	    workingSetMB?: number;
	    virtualSizeMB?: number;
	    peakWorkingSetMB?: number;
	    sharedMemoryMB?: number;
	    dataMemoryMB?: number;
	    readCount: number;
	    writeCount: number;
	    readBytesMB?: number;
	    writeBytesMB?: number;
	    cpuPercent?: number;
	    totalCPUSeconds?: number;
	    userModeTimeSec?: number;
	    kernelModeTimeSec?: number;
	    isRunning?: boolean;
	    foreground?: boolean;
	    fileSizeMB?: number;
	    fileCreated: string;
	    fileModified: string;
	    fileProductName: string;
	    fileProductVersion: string;
	    fileVersion: string;
	    fileCompany: string;
	    fileDescription: string;
	    sha256: string;
	    tcpConnections: string;
	
	    static createFrom(source: any = {}) {
	        return new CodexProcessInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pid = source["pid"];
	        this.name = source["name"];
	        this.commandLine = source["commandLine"];
	        this.executablePath = source["executablePath"];
	        this.owner = source["owner"];
	        this.creationDate = source["creationDate"];
	        this.parentPid = source["parentPid"];
	        this.parentName = source["parentName"];
	        this.parentCommandLine = source["parentCommandLine"];
	        this.childProcesses = source["childProcesses"];
	        this.status = source["status"];
	        this.threadCount = source["threadCount"];
	        this.handleCount = source["handleCount"];
	        this.workingSetMB = source["workingSetMB"];
	        this.virtualSizeMB = source["virtualSizeMB"];
	        this.peakWorkingSetMB = source["peakWorkingSetMB"];
	        this.sharedMemoryMB = source["sharedMemoryMB"];
	        this.dataMemoryMB = source["dataMemoryMB"];
	        this.readCount = source["readCount"];
	        this.writeCount = source["writeCount"];
	        this.readBytesMB = source["readBytesMB"];
	        this.writeBytesMB = source["writeBytesMB"];
	        this.cpuPercent = source["cpuPercent"];
	        this.totalCPUSeconds = source["totalCPUSeconds"];
	        this.userModeTimeSec = source["userModeTimeSec"];
	        this.kernelModeTimeSec = source["kernelModeTimeSec"];
	        this.isRunning = source["isRunning"];
	        this.foreground = source["foreground"];
	        this.fileSizeMB = source["fileSizeMB"];
	        this.fileCreated = source["fileCreated"];
	        this.fileModified = source["fileModified"];
	        this.fileProductName = source["fileProductName"];
	        this.fileProductVersion = source["fileProductVersion"];
	        this.fileVersion = source["fileVersion"];
	        this.fileCompany = source["fileCompany"];
	        this.fileDescription = source["fileDescription"];
	        this.sha256 = source["sha256"];
	        this.tcpConnections = source["tcpConnections"];
	    }
	}
	export class EnvironmentConfig {
	    codexAuthPath: string;
	    codexAccountId: string;
	    codexEmail: string;
	    codexSubscription: string;
	    codexWorkspaceName: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new EnvironmentConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.codexAuthPath = source["codexAuthPath"];
	        this.codexAccountId = source["codexAccountId"];
	        this.codexEmail = source["codexEmail"];
	        this.codexSubscription = source["codexSubscription"];
	        this.codexWorkspaceName = source["codexWorkspaceName"];
	        this.updatedAt = source["updatedAt"];
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

