<script lang="ts">
	// The weekly-hours editor. Without eventTypeId it edits the host's global hours
	// (Availability page); with it, the hours that apply to that event type only
	// (event type → Availability tab). Both talk to /v1/availability-rules.
	import { onMount } from 'svelte';
	import { api, type AvailabilityRule } from '$lib/api';
	import { prefs } from '$lib/prefs';
	import { buttonVariants } from '$lib/components/ui/button';
	import * as Tooltip from '$lib/components/ui/tooltip';
	import * as Select from '$lib/components/ui/select';

	let { eventTypeId = undefined }: { eventTypeId?: string } = $props();

	const DAY_NAMES = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];

	const TIME_SLOTS: string[] = [];
	for (let h = 0; h < 24; h++) {
		for (const m of [0, 30]) {
			TIME_SLOTS.push(`${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}`);
		}
	}

	function fmtTime(t: string) {
		const [hh, mm] = t.split(':').map(Number);
		if ($prefs.time_format === '24h') {
			return mm === 0 ? `${String(hh).padStart(2,'0')}:00` : `${String(hh).padStart(2,'0')}:${String(mm).padStart(2,'0')}`;
		}
		const ampm = hh < 12 ? 'am' : 'pm';
		const h12 = hh % 12 || 12;
		return mm === 0 ? `${h12}${ampm}` : `${h12}:${String(mm).padStart(2,'0')}${ampm}`;
	}

	// ── Weekly rules — 7-day model ────────────────────────────────────────────────
	type DayBlock = { id: string; start_time: string; end_time: string; saving: boolean; error: string }
	type DayState = { day_of_week: number; blocks: DayBlock[]; error: string }

	let days: DayState[] = $state(
		Array.from({ length: 7 }, (_, i) => ({ day_of_week: i, blocks: [], error: '' }))
	);

	let rulesLoading = $state(true);
	let rulesError = $state('');

	let orderedDays = $derived([...days].sort((a, b) => {
		const ws = $prefs.week_start ?? 1;
		return ((a.day_of_week - ws + 7) % 7) - ((b.day_of_week - ws + 7) % 7);
	}));

	async function loadRules() {
		rulesError = '';
		rulesLoading = true;
		try {
			const res = await api.get<{ items: AvailabilityRule[] }>(
				eventTypeId ? `/v1/availability-rules?event_type_id=${encodeURIComponent(eventTypeId)}` : '/v1/availability-rules'
			);
			for (const day of days) { day.blocks = []; day.error = ''; }
			for (const r of (res.items ?? [])) {
				const day = days[r.day_of_week];
				if (day) day.blocks.push({ id: r.id, start_time: r.start_time, end_time: r.end_time, saving: false, error: '' });
			}
			for (const day of days) {
				day.blocks.sort((a, b) => a.start_time.localeCompare(b.start_time));
			}
		} catch (e: any) {
			rulesError = e.message;
		} finally {
			rulesLoading = false;
		}
	}

	async function addBlock(day: DayState) {
		day.blocks.push({ id: '', start_time: '09:00', end_time: '17:00', saving: true, error: '' });
		// Operate on the element AS STORED in the reactive array (a proxy), not the literal we
		// pushed — Svelte 5 proxies pushed objects, so mutating the local literal wouldn't update
		// the UI (the "saving…" stuck-until-refresh bug).
		const block = day.blocks[day.blocks.length - 1];
		try {
			const r = await api.post<AvailabilityRule>('/v1/availability-rules', {
				...(eventTypeId ? { event_type_id: eventTypeId } : {}),
				day_of_week: day.day_of_week,
				start_time: '09:00',
				end_time: '17:00'
			});
			block.id = r.id;
		} catch (e: any) {
			const idx = day.blocks.indexOf(block);
			if (idx !== -1) day.blocks.splice(idx, 1);
			day.error = e.message;
		} finally {
			block.saving = false;
		}
	}

	async function updateBlock(block: DayBlock) {
		if (!block.id) return;
		if (block.start_time >= block.end_time) {
			block.error = 'End must be after start';
			return;
		}
		block.error = '';
		block.saving = true;
		try {
			await api.patch(`/v1/availability-rules/${block.id}`, {
				start_time: block.start_time,
				end_time: block.end_time
			});
		} catch (e: any) {
			block.error = e.message;
		} finally {
			block.saving = false;
		}
	}

	async function removeBlock(day: DayState, block: DayBlock) {
		if (!block.id) return;
		block.saving = true;
		try {
			await api.del(`/v1/availability-rules/${block.id}`);
			const idx = day.blocks.indexOf(block);
			if (idx !== -1) day.blocks.splice(idx, 1);
		} catch (e: any) {
			block.error = e.message;
			block.saving = false;
		}
	}

	let timeOptions = $derived(TIME_SLOTS.map((t) => ({ value: t, label: fmtTime(t) })));
	function timeLabel(t: string) { return fmtTime(t); }

	onMount(() => {
		loadRules();
	});
</script>

	{#if rulesError}<p class="mb-3 rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">{rulesError}</p>{/if}

{#if rulesLoading}
	<p class="py-4 text-sm text-muted-foreground">Loading…</p>
{:else}
	<div class="rounded-lg border bg-card">
		<Tooltip.Provider>
			{#each orderedDays as day, i}
				<div class="flex gap-4 px-4 py-3 {i > 0 ? 'border-t' : ''}">
					<!-- Day name -->
					<div class="w-24 shrink-0 pt-1.5 text-sm {day.blocks.length === 0 ? 'font-normal text-muted-foreground' : 'font-medium'}">
						{DAY_NAMES[day.day_of_week]}
					</div>

					<!-- Blocks -->
					<div class="flex-1 space-y-2">
						{#if day.error}
							<p class="text-xs text-destructive">{day.error}</p>
						{/if}

						{#if day.blocks.length === 0}
							<div class="flex items-center gap-3 py-0.5">
								<span class="text-sm text-muted-foreground/50">No hours set</span>
								<button
									onclick={() => addBlock(day)}
									class="text-sm text-primary hover:underline"
								>
									+ Add hours
								</button>
							</div>
						{:else}
							{#each day.blocks as block}
								<div class="flex items-center gap-2">
									<Select.Root
										type="single"
										bind:value={block.start_time}
										onValueChange={() => updateBlock(block)}
										disabled={block.saving || !block.id}
									>
										<Select.Trigger class="w-fit">{timeLabel(block.start_time)}</Select.Trigger>
										<Select.Content>
											{#each timeOptions as t}<Select.Item value={t.value} label={t.label}>{t.label}</Select.Item>{/each}
										</Select.Content>
									</Select.Root>
									<span class="text-muted-foreground">–</span>
									<Select.Root
										type="single"
										bind:value={block.end_time}
										onValueChange={() => updateBlock(block)}
										disabled={block.saving || !block.id}
									>
										<Select.Trigger class="w-fit">{timeLabel(block.end_time)}</Select.Trigger>
										<Select.Content>
											{#each timeOptions as t}<Select.Item value={t.value} label={t.label}>{t.label}</Select.Item>{/each}
										</Select.Content>
									</Select.Root>
									{#if block.saving}
										<span class="text-xs text-muted-foreground">saving…</span>
									{:else if block.error}
										<span class="text-xs text-destructive">{block.error}</span>
									{/if}
									<Tooltip.Root>
										<Tooltip.Trigger
											class={buttonVariants({ variant: 'ghost', size: 'icon' })}
											onclick={() => removeBlock(day, block)}
											disabled={block.saving}
										>
											<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/><path d="M9 6V4h6v2"/></svg>
										</Tooltip.Trigger>
										<Tooltip.Content>Remove</Tooltip.Content>
									</Tooltip.Root>
								</div>
							{/each}
							<button
								onclick={() => addBlock(day)}
								class="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
							>
								<svg xmlns="http://www.w3.org/2000/svg" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
								Add block
							</button>
						{/if}
					</div>
				</div>
			{/each}
		</Tooltip.Provider>
	</div>
{/if}
