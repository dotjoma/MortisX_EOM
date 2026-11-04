export namespace main {
	
	export class AppSettings {
	    discordRichPresence: boolean;
	    enableLogs: boolean;
	    enableConsole: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AppSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.discordRichPresence = source["discordRichPresence"];
	        this.enableLogs = source["enableLogs"];
	        this.enableConsole = source["enableConsole"];
	    }
	}
	export class RegisterAccountRequest {
	    username: string;
	    password: string;
	    server: string;
	    target_1st_job?: string;
	    target_2nd_job?: string;
	
	    static createFrom(source: any = {}) {
	        return new RegisterAccountRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.username = source["username"];
	        this.password = source["password"];
	        this.server = source["server"];
	        this.target_1st_job = source["target_1st_job"];
	        this.target_2nd_job = source["target_2nd_job"];
	    }
	}

}

export namespace models {
	
	export class BotIntervals {
	    move_sec: number;
	    attack_sec: number;
	    loot_sec: number;
	    skill_sec: number;
	
	    static createFrom(source: any = {}) {
	        return new BotIntervals(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.move_sec = source["move_sec"];
	        this.attack_sec = source["attack_sec"];
	        this.loot_sec = source["loot_sec"];
	        this.skill_sec = source["skill_sec"];
	    }
	}
	export class CustomStatRatio {
	    str_pct: number;
	    sta_pct: number;
	    dex_pct: number;
	    int_pct: number;
	    cap_stat: string;
	    cap_value: number;
	    then_stat: string;
	
	    static createFrom(source: any = {}) {
	        return new CustomStatRatio(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.str_pct = source["str_pct"];
	        this.sta_pct = source["sta_pct"];
	        this.dex_pct = source["dex_pct"];
	        this.int_pct = source["int_pct"];
	        this.cap_stat = source["cap_stat"];
	        this.cap_value = source["cap_value"];
	        this.then_stat = source["then_stat"];
	    }
	}
	export class JobProgressionConfig {
	    auto_job_change: boolean;
	    target_1st_job: string;
	    target_2nd_job: string;
	    auto_rebirth: boolean;
	    stat_mode: string;
	    selected_preset: string;
	    custom_ratio: CustomStatRatio;
	    auto_equip_starter: boolean;
	
	    static createFrom(source: any = {}) {
	        return new JobProgressionConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.auto_job_change = source["auto_job_change"];
	        this.target_1st_job = source["target_1st_job"];
	        this.target_2nd_job = source["target_2nd_job"];
	        this.auto_rebirth = source["auto_rebirth"];
	        this.stat_mode = source["stat_mode"];
	        this.selected_preset = source["selected_preset"];
	        this.custom_ratio = this.convertValues(source["custom_ratio"], CustomStatRatio);
	        this.auto_equip_starter = source["auto_equip_starter"];
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
	export class BotPreferences {
	    bypass_logon: boolean;
	    auto_reconnect: boolean;
	    auto_reconnect_delay?: number;
	    auto_accept_party: boolean;
	    auto_accept_trade: boolean;
	    anti_afk: boolean;
	    auto_disconnect_on_gm: boolean;
	    invisible: boolean;
	    legit_mode: boolean;
	    auto_claim_daily_gifts: boolean;
	    disable_auto_rest: boolean;
	    max_penya_cap?: number;
	    move_step?: number;
	    job_config: JobProgressionConfig;
	    autoEquipUpgrades: boolean;
	    lockWeaponType: boolean;
	    autoEquipArrows: boolean;
	    autoJunkInferior: boolean;
	    autoLockRareGear: boolean;
	    autoManagePet: boolean;
	    autoUsePowerups: boolean;
	    autoRefuelFlight: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BotPreferences(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.bypass_logon = source["bypass_logon"];
	        this.auto_reconnect = source["auto_reconnect"];
	        this.auto_reconnect_delay = source["auto_reconnect_delay"];
	        this.auto_accept_party = source["auto_accept_party"];
	        this.auto_accept_trade = source["auto_accept_trade"];
	        this.anti_afk = source["anti_afk"];
	        this.auto_disconnect_on_gm = source["auto_disconnect_on_gm"];
	        this.invisible = source["invisible"];
	        this.legit_mode = source["legit_mode"];
	        this.auto_claim_daily_gifts = source["auto_claim_daily_gifts"];
	        this.disable_auto_rest = source["disable_auto_rest"];
	        this.max_penya_cap = source["max_penya_cap"];
	        this.move_step = source["move_step"];
	        this.job_config = this.convertValues(source["job_config"], JobProgressionConfig);
	        this.autoEquipUpgrades = source["autoEquipUpgrades"];
	        this.lockWeaponType = source["lockWeaponType"];
	        this.autoEquipArrows = source["autoEquipArrows"];
	        this.autoJunkInferior = source["autoJunkInferior"];
	        this.autoLockRareGear = source["autoLockRareGear"];
	        this.autoManagePet = source["autoManagePet"];
	        this.autoUsePowerups = source["autoUsePowerups"];
	        this.autoRefuelFlight = source["autoRefuelFlight"];
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
	export class BotProfile {
	    id: string;
	    name: string;
	    server: string;
	    character_name: string;
	    preferences: BotPreferences;
	    intervals: BotIntervals;
	    password?: string;
	
	    static createFrom(source: any = {}) {
	        return new BotProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.server = source["server"];
	        this.character_name = source["character_name"];
	        this.preferences = this.convertValues(source["preferences"], BotPreferences);
	        this.intervals = this.convertValues(source["intervals"], BotIntervals);
	        this.password = source["password"];
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
	export class WaypointCoord {
	    x: number;
	    z: number;
	
	    static createFrom(source: any = {}) {
	        return new WaypointCoord(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.x = source["x"];
	        this.z = source["z"];
	    }
	}
	export class BotRuntimeStats {
	    ping_ms: number;
	    played_seconds: number;
	    deaths: number;
	    zone: string;
	    level: number;
	    exp: number;
	    max_exp: number;
	    gold: number;
	    pos_x: number;
	    pos_y: number;
	    pos_z: number;
	    job: string;
	    hp: number;
	    mp: number;
	    fp: number;
	    str: number;
	    sta: number;
	    dex: number;
	    int: number;
	    stat_points: number;
	    auto_stat: number;
	    waypoint?: WaypointCoord;
	
	    static createFrom(source: any = {}) {
	        return new BotRuntimeStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ping_ms = source["ping_ms"];
	        this.played_seconds = source["played_seconds"];
	        this.deaths = source["deaths"];
	        this.zone = source["zone"];
	        this.level = source["level"];
	        this.exp = source["exp"];
	        this.max_exp = source["max_exp"];
	        this.gold = source["gold"];
	        this.pos_x = source["pos_x"];
	        this.pos_y = source["pos_y"];
	        this.pos_z = source["pos_z"];
	        this.job = source["job"];
	        this.hp = source["hp"];
	        this.mp = source["mp"];
	        this.fp = source["fp"];
	        this.str = source["str"];
	        this.sta = source["sta"];
	        this.dex = source["dex"];
	        this.int = source["int"];
	        this.stat_points = source["stat_points"];
	        this.auto_stat = source["auto_stat"];
	        this.waypoint = this.convertValues(source["waypoint"], WaypointCoord);
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
	export class BotSummary {
	    id: string;
	    name: string;
	    server: string;
	    character_name: string;
	    status: string;
	    status_message?: string;
	    stats: BotRuntimeStats;
	    preferences: BotPreferences;
	    intervals: BotIntervals;
	    password?: string;
	
	    static createFrom(source: any = {}) {
	        return new BotSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.server = source["server"];
	        this.character_name = source["character_name"];
	        this.status = source["status"];
	        this.status_message = source["status_message"];
	        this.stats = this.convertValues(source["stats"], BotRuntimeStats);
	        this.preferences = this.convertValues(source["preferences"], BotPreferences);
	        this.intervals = this.convertValues(source["intervals"], BotIntervals);
	        this.password = source["password"];
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
	export class ChannelInfo {
	    id: number;
	    name: string;
	    online: number;
	    max: number;
	    pk: boolean;
	    market: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ChannelInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.online = source["online"];
	        this.max = source["max"];
	        this.pk = source["pk"];
	        this.market = source["market"];
	    }
	}
	
	export class FarmActiveBuff {
	    id: string;
	    name: string;
	    seconds_left: number;
	    icon: string;
	
	    static createFrom(source: any = {}) {
	        return new FarmActiveBuff(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.seconds_left = source["seconds_left"];
	        this.icon = source["icon"];
	    }
	}
	export class FarmConfig {
	    enabled: boolean;
	    mode: string;
	    anchor_x: number;
	    anchor_y: number;
	    anchor_z: number;
	    radius: number;
	    follow_target: string;
	    follow_dist: number;
	    heal_threshold: number;
	    circle_heal: boolean;
	    auto_resurrection: boolean;
	    skills: string[];
	    buffs: string[];
	    support_buffs: string[];
	    auto_attack: boolean;
	    auto_lucky: boolean;
	    auto_amp: boolean;
	    auto_upcut: boolean;
	    auto_def_upcut: boolean;
	    auto_grilled_eel: boolean;
	    auto_potions: boolean;
	    hp_threshold: number;
	    mp_threshold: number;
	    fp_threshold: number;
	    auto_rest: boolean;
	    rest_hp_threshold: number;
	    auto_loot: boolean;
	    loot_rarity_min: number;
	    loot_penya_only: boolean;
	    loot_quest_items: boolean;
	    stop_when_bag_full: boolean;
	    smart_mode: boolean;
	    smart_min_level_offset: number;
	    smart_max_level_offset: number;
	    avoid_kill_steal: boolean;
	    evacuate_on_player_nearby: boolean;
	    player_detection_radius: number;
	    ghost_mode_block_rec_put: boolean;
	    exploit_one_kill: boolean;
	    exploit_multi_sweep: boolean;
	    exploit_god_mode: boolean;
	    exploit_cap_hp1: boolean;
	    coordinated_farm: boolean;
	    watchdog_sec: number;
	    discord_webhook_url: string;
	    webhook_alert_on_player: boolean;
	    webhook_alert_on_death: boolean;
	    webhook_alert_on_rare_drop: boolean;
	    webhook_alert_on_bag_full: boolean;
	    webhook_alert_on_disconnect: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FarmConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.mode = source["mode"];
	        this.anchor_x = source["anchor_x"];
	        this.anchor_y = source["anchor_y"];
	        this.anchor_z = source["anchor_z"];
	        this.radius = source["radius"];
	        this.follow_target = source["follow_target"];
	        this.follow_dist = source["follow_dist"];
	        this.heal_threshold = source["heal_threshold"];
	        this.circle_heal = source["circle_heal"];
	        this.auto_resurrection = source["auto_resurrection"];
	        this.skills = source["skills"];
	        this.buffs = source["buffs"];
	        this.support_buffs = source["support_buffs"];
	        this.auto_attack = source["auto_attack"];
	        this.auto_lucky = source["auto_lucky"];
	        this.auto_amp = source["auto_amp"];
	        this.auto_upcut = source["auto_upcut"];
	        this.auto_def_upcut = source["auto_def_upcut"];
	        this.auto_grilled_eel = source["auto_grilled_eel"];
	        this.auto_potions = source["auto_potions"];
	        this.hp_threshold = source["hp_threshold"];
	        this.mp_threshold = source["mp_threshold"];
	        this.fp_threshold = source["fp_threshold"];
	        this.auto_rest = source["auto_rest"];
	        this.rest_hp_threshold = source["rest_hp_threshold"];
	        this.auto_loot = source["auto_loot"];
	        this.loot_rarity_min = source["loot_rarity_min"];
	        this.loot_penya_only = source["loot_penya_only"];
	        this.loot_quest_items = source["loot_quest_items"];
	        this.stop_when_bag_full = source["stop_when_bag_full"];
	        this.smart_mode = source["smart_mode"];
	        this.smart_min_level_offset = source["smart_min_level_offset"];
	        this.smart_max_level_offset = source["smart_max_level_offset"];
	        this.avoid_kill_steal = source["avoid_kill_steal"];
	        this.evacuate_on_player_nearby = source["evacuate_on_player_nearby"];
	        this.player_detection_radius = source["player_detection_radius"];
	        this.ghost_mode_block_rec_put = source["ghost_mode_block_rec_put"];
	        this.exploit_one_kill = source["exploit_one_kill"];
	        this.exploit_multi_sweep = source["exploit_multi_sweep"];
	        this.exploit_god_mode = source["exploit_god_mode"];
	        this.exploit_cap_hp1 = source["exploit_cap_hp1"];
	        this.coordinated_farm = source["coordinated_farm"];
	        this.watchdog_sec = source["watchdog_sec"];
	        this.discord_webhook_url = source["discord_webhook_url"];
	        this.webhook_alert_on_player = source["webhook_alert_on_player"];
	        this.webhook_alert_on_death = source["webhook_alert_on_death"];
	        this.webhook_alert_on_rare_drop = source["webhook_alert_on_rare_drop"];
	        this.webhook_alert_on_bag_full = source["webhook_alert_on_bag_full"];
	        this.webhook_alert_on_disconnect = source["webhook_alert_on_disconnect"];
	    }
	}
	export class FarmLootFeedItem {
	    timestamp: string;
	    item_name: string;
	    item_id: string;
	    count: number;
	    rarity: number;
	    is_penya: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FarmLootFeedItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timestamp = source["timestamp"];
	        this.item_name = source["item_name"];
	        this.item_id = source["item_id"];
	        this.count = source["count"];
	        this.rarity = source["rarity"];
	        this.is_penya = source["is_penya"];
	    }
	}
	export class FarmStats {
	    running: boolean;
	    state: string;
	    target_name: string;
	    target_hp: number;
	    target_max_hp: number;
	    kills: number;
	    kills_per_hour: number;
	    exp_gained: number;
	    exp_per_hour: number;
	    time_to_level_sec: number;
	    penya_gained: number;
	    penya_per_hour: number;
	    session_sec: number;
	    recent_loot: FarmLootFeedItem[];
	    active_buffs: FarmActiveBuff[];
	    coord_claimed_mob: number;
	    coord_zone: string;
	    coord_zone_mobs: string;
	    coord_assigned_at: string;
	
	    static createFrom(source: any = {}) {
	        return new FarmStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.state = source["state"];
	        this.target_name = source["target_name"];
	        this.target_hp = source["target_hp"];
	        this.target_max_hp = source["target_max_hp"];
	        this.kills = source["kills"];
	        this.kills_per_hour = source["kills_per_hour"];
	        this.exp_gained = source["exp_gained"];
	        this.exp_per_hour = source["exp_per_hour"];
	        this.time_to_level_sec = source["time_to_level_sec"];
	        this.penya_gained = source["penya_gained"];
	        this.penya_per_hour = source["penya_per_hour"];
	        this.session_sec = source["session_sec"];
	        this.recent_loot = this.convertValues(source["recent_loot"], FarmLootFeedItem);
	        this.active_buffs = this.convertValues(source["active_buffs"], FarmActiveBuff);
	        this.coord_claimed_mob = source["coord_claimed_mob"];
	        this.coord_zone = source["coord_zone"];
	        this.coord_zone_mobs = source["coord_zone_mobs"];
	        this.coord_assigned_at = source["coord_assigned_at"];
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
	
	export class NearbyPlayerInfo {
	    id: number;
	    name: string;
	    x: number;
	    y: number;
	    z: number;
	
	    static createFrom(source: any = {}) {
	        return new NearbyPlayerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.x = source["x"];
	        this.y = source["y"];
	        this.z = source["z"];
	    }
	}
	export class PartyBoostConfig {
	    enabled: boolean;
	    leader_name: string;
	    selected_members: string[];
	    auto_reinvite: boolean;
	    leash_enabled: boolean;
	    leash_distance: number;
	    party_name: string;
	    auto_advance: boolean;
	    exp_share_mode: number;
	    item_dist_mode: number;
	    level_sync: boolean;
	    focus_fire: boolean;
	    auto_support: boolean;
	    skills_enabled: Record<string, boolean>;
	    scrolls_enabled: Record<string, boolean>;
	
	    static createFrom(source: any = {}) {
	        return new PartyBoostConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.leader_name = source["leader_name"];
	        this.selected_members = source["selected_members"];
	        this.auto_reinvite = source["auto_reinvite"];
	        this.leash_enabled = source["leash_enabled"];
	        this.leash_distance = source["leash_distance"];
	        this.party_name = source["party_name"];
	        this.auto_advance = source["auto_advance"];
	        this.exp_share_mode = source["exp_share_mode"];
	        this.item_dist_mode = source["item_dist_mode"];
	        this.level_sync = source["level_sync"];
	        this.focus_fire = source["focus_fire"];
	        this.auto_support = source["auto_support"];
	        this.skills_enabled = source["skills_enabled"];
	        this.scrolls_enabled = source["scrolls_enabled"];
	    }
	}
	export class PartyRosterEntry {
	    bot_id: string;
	    account_name: string;
	    character_name: string;
	    level: number;
	    job: string;
	    channel: string;
	    status: string;
	    in_party: boolean;
	    party_id?: number;
	    party_name?: string;
	    party_leader?: string;
	    party_level?: number;
	    party_kind?: number;
	    party_share?: number;
	    party_dist?: number;
	    party_points?: number;
	    party_scroll_full?: boolean;
	    party_scroll_exp_up?: boolean;
	    party_scroll_skill_up?: boolean;
	    party_modes?: Record<string, number>;
	    party_skill_cd?: Record<string, number>;
	    is_leader: boolean;
	    distance_to_leader: number;
	    in_share_range: boolean;
	    level_sync_active?: boolean;
	    sync_level?: number;
	
	    static createFrom(source: any = {}) {
	        return new PartyRosterEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.bot_id = source["bot_id"];
	        this.account_name = source["account_name"];
	        this.character_name = source["character_name"];
	        this.level = source["level"];
	        this.job = source["job"];
	        this.channel = source["channel"];
	        this.status = source["status"];
	        this.in_party = source["in_party"];
	        this.party_id = source["party_id"];
	        this.party_name = source["party_name"];
	        this.party_leader = source["party_leader"];
	        this.party_level = source["party_level"];
	        this.party_kind = source["party_kind"];
	        this.party_share = source["party_share"];
	        this.party_dist = source["party_dist"];
	        this.party_points = source["party_points"];
	        this.party_scroll_full = source["party_scroll_full"];
	        this.party_scroll_exp_up = source["party_scroll_exp_up"];
	        this.party_scroll_skill_up = source["party_scroll_skill_up"];
	        this.party_modes = source["party_modes"];
	        this.party_skill_cd = source["party_skill_cd"];
	        this.is_leader = source["is_leader"];
	        this.distance_to_leader = source["distance_to_leader"];
	        this.in_share_range = source["in_share_range"];
	        this.level_sync_active = source["level_sync_active"];
	        this.sync_level = source["sync_level"];
	    }
	}

}

export namespace multi {
	
	export class BotRef {
	    id: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new BotRef(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class BotResult {
	    botId: string;
	    status: number;
	    reason?: string;
	    data?: any;
	
	    static createFrom(source: any = {}) {
	        return new BotResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.botId = source["botId"];
	        this.status = source["status"];
	        this.reason = source["reason"];
	        this.data = source["data"];
	    }
	}
	export class ConfirmRequest {
	    entry: string;
	    bots: BotRef[];
	    items?: string[];
	
	    static createFrom(source: any = {}) {
	        return new ConfirmRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entry = source["entry"];
	        this.bots = this.convertValues(source["bots"], BotRef);
	        this.items = source["items"];
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
	export class FieldSchema {
	    key: string;
	    kind: string;
	    type: string;
	    default?: any;
	    options?: string[];
	    min?: number;
	    max?: number;
	    risk: string;
	    scope: string;
	    sensitive: boolean;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new FieldSchema(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.kind = source["kind"];
	        this.type = source["type"];
	        this.default = source["default"];
	        this.options = source["options"];
	        this.min = source["min"];
	        this.max = source["max"];
	        this.risk = source["risk"];
	        this.scope = source["scope"];
	        this.sensitive = source["sensitive"];
	        this.label = source["label"];
	    }
	}
	export class ModuleInfo {
	    tab: string;
	    id: string;
	    title: string;
	    order: number;
	    icon: string;
	    fields: FieldSchema[];
	
	    static createFrom(source: any = {}) {
	        return new ModuleInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tab = source["tab"];
	        this.id = source["id"];
	        this.title = source["title"];
	        this.order = source["order"];
	        this.icon = source["icon"];
	        this.fields = this.convertValues(source["fields"], FieldSchema);
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
	export class Opts {
	    confirm: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Opts(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.confirm = source["confirm"];
	    }
	}
	export class Summary {
	    total: number;
	    ok: number;
	    skipped: number;
	    failed: number;
	    results: BotResult[];
	
	    static createFrom(source: any = {}) {
	        return new Summary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.ok = source["ok"];
	        this.skipped = source["skipped"];
	        this.failed = source["failed"];
	        this.results = this.convertValues(source["results"], BotResult);
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
	export class Result {
	    summary: Summary;
	    confirm?: ConfirmRequest;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.summary = this.convertValues(source["summary"], Summary);
	        this.confirm = this.convertValues(source["confirm"], ConfirmRequest);
	        this.error = source["error"];
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
	export class Schema {
	    modules: ModuleInfo[];
	
	    static createFrom(source: any = {}) {
	        return new Schema(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.modules = this.convertValues(source["modules"], ModuleInfo);
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
	
	export class Target {
	    ids: string[];
	    focusedId: string;
	
	    static createFrom(source: any = {}) {
	        return new Target(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ids = source["ids"];
	        this.focusedId = source["focusedId"];
	    }
	}

}

export namespace vpn {
	
	export class Status {
	    state: string;
	    server?: string;
	    error?: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.server = source["server"];
	        this.error = source["error"];
	        this.message = source["message"];
	    }
	}

}

