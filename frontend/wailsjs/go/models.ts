export namespace history {
	
	export class Entry {
	    path: string;
	    filename: string;
	    title: string;
	    position: number;
	    duration: number;
	    // Go type: time
	    updatedAt: any;
	    exists: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.filename = source["filename"];
	        this.title = source["title"];
	        this.position = source["position"];
	        this.duration = source["duration"];
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
	        this.exists = source["exists"];
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
	
	export class Diagnostics {
	    appName: string;
	    tagline: string;
	    engineReady: boolean;
	    startupError: string;
	    mpvPath: string;
	    ffmpegPath: string;
	    ffprobePath: string;
	    searchDirs: string[];
	    settingsPath: string;
	    historyPath: string;
	    playlistPath: string;
	    hwdec: string;
	
	    static createFrom(source: any = {}) {
	        return new Diagnostics(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.appName = source["appName"];
	        this.tagline = source["tagline"];
	        this.engineReady = source["engineReady"];
	        this.startupError = source["startupError"];
	        this.mpvPath = source["mpvPath"];
	        this.ffmpegPath = source["ffmpegPath"];
	        this.ffprobePath = source["ffprobePath"];
	        this.searchDirs = source["searchDirs"];
	        this.settingsPath = source["settingsPath"];
	        this.historyPath = source["historyPath"];
	        this.playlistPath = source["playlistPath"];
	        this.hwdec = source["hwdec"];
	    }
	}
	export class OpenResult {
	    path: string;
	    filename: string;
	    resumeAvailable: boolean;
	    resumePosition: number;
	    autoResumed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new OpenResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.filename = source["filename"];
	        this.resumeAvailable = source["resumeAvailable"];
	        this.resumePosition = source["resumePosition"];
	        this.autoResumed = source["autoResumed"];
	    }
	}
	export class TranscriptResult {
	    entries: transcript.Entry[];
	    trackId: number;
	    trackLabel: string;
	    source: string;
	    available: boolean;
	    status: string;
	
	    static createFrom(source: any = {}) {
	        return new TranscriptResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entries = this.convertValues(source["entries"], transcript.Entry);
	        this.trackId = source["trackId"];
	        this.trackLabel = source["trackLabel"];
	        this.source = source["source"];
	        this.available = source["available"];
	        this.status = source["status"];
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

export namespace player {
	
	export class AudioInfo {
	    id: number;
	    codec: string;
	    codecLong: string;
	    language: string;
	    title: string;
	    channels: number;
	    channelLayout: string;
	    sampleRate: number;
	    bitrate: number;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new AudioInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.codec = source["codec"];
	        this.codecLong = source["codecLong"];
	        this.language = source["language"];
	        this.title = source["title"];
	        this.channels = source["channels"];
	        this.channelLayout = source["channelLayout"];
	        this.sampleRate = source["sampleRate"];
	        this.bitrate = source["bitrate"];
	        this.label = source["label"];
	    }
	}
	export class Chapter {
	    index: number;
	    title: string;
	    start: number;
	
	    static createFrom(source: any = {}) {
	        return new Chapter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.title = source["title"];
	        this.start = source["start"];
	    }
	}
	export class Track {
	    id: number;
	    type: string;
	    language: string;
	    title: string;
	    codec: string;
	    selected: boolean;
	    default: boolean;
	    external: boolean;
	    externalFilename: string;
	    ffIndex: number;
	    channels: number;
	    sampleRate: number;
	    width: number;
	    height: number;
	    fps: number;
	    imageBased: boolean;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new Track(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.language = source["language"];
	        this.title = source["title"];
	        this.codec = source["codec"];
	        this.selected = source["selected"];
	        this.default = source["default"];
	        this.external = source["external"];
	        this.externalFilename = source["externalFilename"];
	        this.ffIndex = source["ffIndex"];
	        this.channels = source["channels"];
	        this.sampleRate = source["sampleRate"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.fps = source["fps"];
	        this.imageBased = source["imageBased"];
	        this.label = source["label"];
	    }
	}
	export class SubtitleInfo {
	    id: number;
	    codec: string;
	    language: string;
	    title: string;
	    external: boolean;
	    imageBased: boolean;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new SubtitleInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.codec = source["codec"];
	        this.language = source["language"];
	        this.title = source["title"];
	        this.external = source["external"];
	        this.imageBased = source["imageBased"];
	        this.label = source["label"];
	    }
	}
	export class VideoInfo {
	    codec: string;
	    codecLong: string;
	    profile: string;
	    width: number;
	    height: number;
	    fps: number;
	    bitrate: number;
	    pixelFormat: string;
	
	    static createFrom(source: any = {}) {
	        return new VideoInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.codec = source["codec"];
	        this.codecLong = source["codecLong"];
	        this.profile = source["profile"];
	        this.width = source["width"];
	        this.height = source["height"];
	        this.fps = source["fps"];
	        this.bitrate = source["bitrate"];
	        this.pixelFormat = source["pixelFormat"];
	    }
	}
	export class MediaInfo {
	    path: string;
	    filename: string;
	    directory: string;
	    fileSize: number;
	    duration: number;
	    container: string;
	    containerLong: string;
	    title: string;
	    creationDate: string;
	    overallBitrate: number;
	    video: VideoInfo;
	    hasVideo: boolean;
	    audio: AudioInfo[];
	    subtitles: SubtitleInfo[];
	    chapters: Chapter[];
	    tracks: Track[];
	    videoTrackCount: number;
	    audioTrackCount: number;
	    subtitleTrackCount: number;
	    probeError: string;
	
	    static createFrom(source: any = {}) {
	        return new MediaInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.filename = source["filename"];
	        this.directory = source["directory"];
	        this.fileSize = source["fileSize"];
	        this.duration = source["duration"];
	        this.container = source["container"];
	        this.containerLong = source["containerLong"];
	        this.title = source["title"];
	        this.creationDate = source["creationDate"];
	        this.overallBitrate = source["overallBitrate"];
	        this.video = this.convertValues(source["video"], VideoInfo);
	        this.hasVideo = source["hasVideo"];
	        this.audio = this.convertValues(source["audio"], AudioInfo);
	        this.subtitles = this.convertValues(source["subtitles"], SubtitleInfo);
	        this.chapters = this.convertValues(source["chapters"], Chapter);
	        this.tracks = this.convertValues(source["tracks"], Track);
	        this.videoTrackCount = source["videoTrackCount"];
	        this.audioTrackCount = source["audioTrackCount"];
	        this.subtitleTrackCount = source["subtitleTrackCount"];
	        this.probeError = source["probeError"];
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
	export class PlaybackState {
	    position: number;
	    duration: number;
	    paused: boolean;
	    muted: boolean;
	    volume: number;
	    speed: number;
	    reverse: boolean;
	    chapterIndex: number;
	    fileLoaded: boolean;
	    path: string;
	    title: string;
	    idle: boolean;
	    seeking: boolean;
	    eof: boolean;
	    subtitleId: number;
	    audioId: number;
	    subtitleDelay: number;
	    audioDelay: number;
	    hwdec: string;
	
	    static createFrom(source: any = {}) {
	        return new PlaybackState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.position = source["position"];
	        this.duration = source["duration"];
	        this.paused = source["paused"];
	        this.muted = source["muted"];
	        this.volume = source["volume"];
	        this.speed = source["speed"];
	        this.reverse = source["reverse"];
	        this.chapterIndex = source["chapterIndex"];
	        this.fileLoaded = source["fileLoaded"];
	        this.path = source["path"];
	        this.title = source["title"];
	        this.idle = source["idle"];
	        this.seeking = source["seeking"];
	        this.eof = source["eof"];
	        this.subtitleId = source["subtitleId"];
	        this.audioId = source["audioId"];
	        this.subtitleDelay = source["subtitleDelay"];
	        this.audioDelay = source["audioDelay"];
	        this.hwdec = source["hwdec"];
	    }
	}
	
	

}

export namespace playlist {
	
	export class Item {
	    path: string;
	    filename: string;
	    title: string;
	    duration: number;
	    missing: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Item(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.filename = source["filename"];
	        this.title = source["title"];
	        this.duration = source["duration"];
	        this.missing = source["missing"];
	    }
	}
	export class State {
	    items: Item[];
	    current: number;
	    shuffle: boolean;
	    repeat: string;
	
	    static createFrom(source: any = {}) {
	        return new State(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], Item);
	        this.current = source["current"];
	        this.shuffle = source["shuffle"];
	        this.repeat = source["repeat"];
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

export namespace settings {
	
	export class WindowState {
	    width: number;
	    height: number;
	    x: number;
	    y: number;
	    maximised: boolean;
	    valid: boolean;
	
	    static createFrom(source: any = {}) {
	        return new WindowState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.width = source["width"];
	        this.height = source["height"];
	        this.x = source["x"];
	        this.y = source["y"];
	        this.maximised = source["maximised"];
	        this.valid = source["valid"];
	    }
	}
	export class Settings {
	    volume: number;
	    muted: boolean;
	    playbackSpeed: number;
	    autoResume: boolean;
	    followTranscript: boolean;
	    sidePanelVisible: boolean;
	    sidePanelWidth: number;
	    lastSubtitleLang: string;
	    lastAudioLang: string;
	    subtitleDelay: number;
	    audioDelay: number;
	    window: WindowState;
	    logLevel: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.volume = source["volume"];
	        this.muted = source["muted"];
	        this.playbackSpeed = source["playbackSpeed"];
	        this.autoResume = source["autoResume"];
	        this.followTranscript = source["followTranscript"];
	        this.sidePanelVisible = source["sidePanelVisible"];
	        this.sidePanelWidth = source["sidePanelWidth"];
	        this.lastSubtitleLang = source["lastSubtitleLang"];
	        this.lastAudioLang = source["lastAudioLang"];
	        this.subtitleDelay = source["subtitleDelay"];
	        this.audioDelay = source["audioDelay"];
	        this.window = this.convertValues(source["window"], WindowState);
	        this.logLevel = source["logLevel"];
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

export namespace transcript {
	
	export class Entry {
	    start: number;
	    end: number;
	    text: string;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.start = source["start"];
	        this.end = source["end"];
	        this.text = source["text"];
	    }
	}

}

