export type KindCode = 'ean13' | 'ean8' | 'isbn13' | 'mac' | 'sequence' | 'custom';
export type Status = 'reserved' | 'assigned' | 'released' | 'blocked';
export type ReusePolicy = 'never' | 'cooldown' | 'immediate';

export interface KindInfo {
	code: KindCode;
	name: string;
	description: string;
	base_digits: number;
	check_digits: number;
	radix: 10 | 16;
	example: string;
	default_reuse_policy: ReusePolicy;
	custom_kind?: string; // yalnızca özel tiplerde
	density?: number; // kurallara uyan gövdelerin tahmini oranı
}

export interface CustomRule {
	expr: string;
	message: string;
}

export interface CustomKindInput {
	code: string;
	name: string;
	description: string;
	body_digits: number;
	check_digits: number;
	rules: CustomRule[];
	check_expr: string;
}

export interface CustomKind extends CustomKindInput {
	id: number;
	density: number;
	example: string;
	pools: number;
	created_at: string;
}

export interface CustomKindTest {
	ok: boolean;
	error?: string;
	density?: number;
	examples?: string[];
	value?: ValidationResult;
}

export interface PoolStats {
	capacity: number;
	capacity_exact: boolean;
	assigned: number;
	reserved: number;
	released: number;
	reusable: number;
	blocked: number;
	available: number;
	low: boolean;
}

export interface Range {
	id: number;
	pool_id: number;
	start: number;
	end: number;
	next: number;
	size: number;
	valid: number;
	valid_exact: boolean;
	untouched: number;
	first_value: string;
	last_value: string;
	created_at: string;
}

export interface Pool {
	id: number;
	code: string;
	name: string;
	description: string;
	kind: KindCode;
	custom_kind: string | null;
	kind_name: string;
	prefix: string;
	pad_width: number;
	reuse_policy: ReusePolicy;
	cooldown_days: number;
	reservation_ttl_seconds: number;
	low_watermark: number;
	active: boolean;
	created_at: string;
	updated_at: string;
	stats: PoolStats;
	ranges?: Range[];
}

export interface Allocation {
	id: number;
	pool_id: number;
	pool_code: string;
	pool_name: string;
	kind: KindCode;
	kind_name: string;
	value: string;
	status: Status;
	object_type: string | null;
	object_ref: string | null;
	note: string;
	metadata: Record<string, unknown>;
	reserved_until: string | null;
	reusable_at: string | null;
	assigned_at: string | null;
	released_at: string | null;
	created_at: string;
	updated_at: string;
}

export interface AllocationEvent {
	id: number;
	allocation_id: number;
	pool_id: number;
	pool_code: string;
	value: string;
	action: string;
	from_status: Status | null;
	to_status: Status;
	object_type: string | null;
	object_ref: string | null;
	actor: string;
	note: string;
	created_at: string;
}

export interface AllocationDetail extends Allocation {
	events: AllocationEvent[];
}

export interface AllocationPage {
	items: Allocation[];
	total: number;
}

export interface AllocateRequest {
	object_type: string;
	object_ref: string;
	mode?: 'assign' | 'reserve';
	value?: string;
	ttl_seconds?: number;
	note?: string;
	metadata?: Record<string, unknown>;
}

export interface RangeInput {
	start?: number;
	end?: number;
	prefix?: string;
}

export interface CreatePoolInput {
	code: string;
	name: string;
	description?: string;
	kind: KindCode;
	custom_kind?: string;
	prefix?: string;
	pad_width?: number;
	reuse_policy?: ReusePolicy;
	cooldown_days?: number;
	reservation_ttl_seconds?: number;
	low_watermark?: number;
	ranges?: RangeInput[];
}

export type UpdatePoolInput = Partial<
	Pick<
		Pool,
		| 'name'
		| 'description'
		| 'reuse_policy'
		| 'cooldown_days'
		| 'reservation_ttl_seconds'
		| 'low_watermark'
		| 'active'
	>
>;

export interface ValidationResult {
	valid: boolean;
	normalized?: string;
	base?: string;
	details?: string;
	error?: string;
}

export interface LookupMatch {
	pool_id: number;
	pool_code: string;
	pool_name: string;
	value: string;
	state: Status | 'available' | 'reusable';
	allocation: Allocation | null;
}
