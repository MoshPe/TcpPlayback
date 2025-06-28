export namespace main {
	
	export class FileInfo {
	    path: string;
	    session: string;
	    fileName: string;
	    size: number;
	    modTime: string;
	    magicWord: string;
	    messagesPerFile: number;
	    maxFileSize: number;
	    messageCount: number;
	    isOpen: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FileInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.session = source["session"];
	        this.fileName = source["fileName"];
	        this.size = source["size"];
	        this.modTime = source["modTime"];
	        this.magicWord = source["magicWord"];
	        this.messagesPerFile = source["messagesPerFile"];
	        this.maxFileSize = source["maxFileSize"];
	        this.messageCount = source["messageCount"];
	        this.isOpen = source["isOpen"];
	    }
	}
	export class LogEntry {
	    // Go type: time
	    time: any;
	    fromIP: string;
	    fromPort: number;
	    toAddress: string;
	    toPort: number;
	    method: string;
	    data: string;
	    bytes: number;
	
	    static createFrom(source: any = {}) {
	        return new LogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = this.convertValues(source["time"], null);
	        this.fromIP = source["fromIP"];
	        this.fromPort = source["fromPort"];
	        this.toAddress = source["toAddress"];
	        this.toPort = source["toPort"];
	        this.method = source["method"];
	        this.data = source["data"];
	        this.bytes = source["bytes"];
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

