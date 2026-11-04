export interface Target {
  ids: string[];
  focusedId: string;
}

export interface Opts {
  confirm?: boolean;
}

export interface BotRef {
  id: string;
  name: string;
}

export interface ConfirmRequest {
  entry: string;
  bots: BotRef[];
  items?: string[];
}

export interface BotResult {
  botId: string;
  status: number; // 0: OK, 1: Skipped, 2: Failed
  reason?: string;
  data?: any;
}

export interface Summary {
  total: number;
  ok: number;
  skipped: number;
  failed: number;
  results: BotResult[];
}

export interface Result {
  summary: Summary;
  confirm?: ConfirmRequest;
  error?: string;
}

export interface MixedValue<T = any> {
  mixed: boolean;
  value?: T;
  perBot?: Record<string, T>;
}

export interface FieldSchema {
  key: string;
  kind: string;
  type: string;
  enumValues?: string[];
  min?: number;
  max?: number;
  risk: string;
  scope: string;
  sensitive: boolean;
  label: string;
}

export interface ModuleInfo {
  tab: string;
  id: string;
  title: string;
  order: number;
  icon: string;
  fields: FieldSchema[];
}

export interface Schema {
  modules: ModuleInfo[];
}
