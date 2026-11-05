export namespace app {
	
	export class AppInfo {
	    Name: string;
	    Version: string;
	    Commit: string;
	    BuildDate: string;
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.Version = source["Version"];
	        this.Commit = source["Commit"];
	        this.BuildDate = source["BuildDate"];
	    }
	}
	export class PingResult {
	    Status: string;
	    Message: string;
	    Database: string;
	    SchemaVersion: number;
	    GoVersion: string;
	
	    static createFrom(source: any = {}) {
	        return new PingResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Status = source["Status"];
	        this.Message = source["Message"];
	        this.Database = source["Database"];
	        this.SchemaVersion = source["SchemaVersion"];
	        this.GoVersion = source["GoVersion"];
	    }
	}

}

