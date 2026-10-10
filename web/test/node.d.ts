declare module "node:test" {
	function test(name: string, fn: () => void | Promise<void>): void;
	export default test;
}

declare module "node:assert/strict" {
	interface Assert {
		ok(value: unknown, message?: string): void;
		equal(actual: unknown, expected: unknown, message?: string): void;
		deepEqual(actual: unknown, expected: unknown, message?: string): void;
		throws(fn: () => void, error?: RegExp | Function, message?: string): void;
	}
	const assert: Assert;
	export default assert;
}
