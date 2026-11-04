<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type AvailabilityRule, type AvailabilityOverride } from '$lib/api';
	import { prefs, fmtDate } from '$lib/prefs';
	import { Button, buttonVariants } from '$lib/components/ui/button';
	import { ConfirmDialog } from '$lib/components/ui/confirm-dialog';
	import { Badge } from '$lib/components/ui/badge';
	import * as Tooltip from '$lib/components/ui/tooltip';
	import * as Select from '$lib/components/ui/select';
	import { DatePicker } from '$lib/components/ui/date-picker';
	import WeeklyHours from '$lib/components/availability/WeeklyHours.svelte';

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

	// ── Date overrides ────────────────────────────────────────────────────────────
	let overrides: AvailabilityOverride[] = $state([]);
	let overridesLoading = $state(true);
	let overridesError = $state('');

	type OverrideReason = 'day_off' | 'out_of_office' | 'custom_hours';
	const REASON_LABELS: Record<OverrideReason, string> = {
		day_off: 'Day off',
		out_of_office: 'Out of office',
		custom_hours: 'Custom hours'
	};

	let ovForm = $state({ date: '', end_date: '', reason: 'day_off' as OverrideReason, start_time: '09:00', end_time: '17:00' });
	let addingOv = $state(false);
	let ovAddError = $state('');

	let deleteOvOpen = $state(false);
	let deleteOvId = $state('');

	let deleteGroupId = $state('');
	let deleteGroupOpen = $state(false);

	// Collapse per-date rows that share a group_id (a multi-day span) into one entry.
	type OvEntry =
		| { kind: 'single'; ov: AvailabilityOverride }
		| { kind: 'span'; group_id: string; reason: OverrideReason; start: string; end: string; days: number };
	const overrideEntries = $derived.by((): OvEntry[] => {
		const groups = new Map<string, AvailabilityOverride[]>();
		const entries: OvEntry[] = [];
		for (const ov of overrides) {
			if (ov.group_id) {
				const arr = groups.get(ov.group_id) ?? [];
				arr.push(ov);
				groups.set(ov.group_id, arr);
			} else {
				entries.push({ kind: 'single', ov });
			}
		}
		for (const [gid, arr] of groups) {
			const dates = arr.map((o) => o.date).sort();
			entries.push({
				kind: 'span',
				group_id: gid,
				reason: (arr[0].reason ?? 'out_of_office') as OverrideReason,
				start: dates[0],
				end: dates[dates.length - 1],
				days: dates.length
			});
		}
		return entries.sort((a, b) =>
			(a.kind === 'single' ? a.ov.date : a.start).localeCompare(b.kind === 'single' ? b.ov.date : b.start)
		);
	});

	async function loadOverrides() {
		overridesError = '';
		try {
			const res = await api.get<{ items: AvailabilityOverride[] }>('/v1/availability-overrides');
			overrides = (res.items ?? []).sort((a, b) => a.date.localeCompare(b.date));
		} catch (e: any) {
			overridesError = e.message;
		} finally {
			overridesLoading = false;
		}
	}

	// Dates whose custom-hours blocks overlap: they merge into one window at slot
	// time, so flag them rather than silently showing two rows that act as one.
	const overlappingOverrideDates = $derived.by((): string[] => {
		const byDate = new Map<string, AvailabilityOverride[]>();
		for (const ov of overrides) {
			if (ov.reason !== 'custom_hours' || !ov.start_time || !ov.end_time) continue;
			const arr = byDate.get(ov.date) ?? [];
			arr.push(ov);
			byDate.set(ov.date, arr);
		}
		const out: string[] = [];
		for (const [date, blocks] of byDate) {
			const sorted = [...blocks].sort((a, b) => (a.start_time ?? '').localeCompare(b.start_time ?? ''));
			for (let i = 1; i < sorted.length; i++) {
				if ((sorted[i].start_time ?? '') < (sorted[i - 1].end_time ?? '')) {
					out.push(date);
					break;
				}
			}
		}
		return out;
	});




	async function addOverride() {
		ovAddError = '';
		if (!ovForm.date) { ovAddError = 'Date is required.'; return; }
		if (ovForm.reason === 'custom_hours' && (!ovForm.start_time || !ovForm.end_time)) {
			ovAddError = 'Start and end time are required for custom hours.'; return;
		}
		if (ovForm.reason === 'custom_hours' && ovForm.start_time >= ovForm.end_time) {
			ovAddError = 'End time must be after start time.'; return;
		}
		if (ovForm.end_date && ovForm.end_date < ovForm.date) {
			ovAddError = 'End date must be on or after the start date.'; return;
		}
		addingOv = true;
		try {
			const isRange = ovForm.reason !== 'custom_hours' && !!ovForm.end_date && ovForm.end_date > ovForm.date;
			await api.post('/v1/availability-overrides', {
				date: ovForm.date,
				reason: ovForm.reason,
				...(ovForm.reason === 'custom_hours'
					? { start_time: ovForm.start_time, end_time: ovForm.end_time }
					: {}),
				...(isRange ? { end_date: ovForm.end_date } : {})
			});
			ovForm = { date: '', end_date: '', reason: 'day_off', start_time: '09:00', end_time: '17:00' };
			await loadOverrides();
		} catch (e: any) {
			ovAddError = e.message;
		} finally {
			addingOv = false;
		}
	}

	function deleteOverride(id: string) {
		deleteOvId = id;
		deleteOvOpen = true;
	}

	async function doDeleteOverride() {
		try {
			await api.del(`/v1/availability-overrides/${deleteOvId}`);
			await loadOverrides();
		} catch (e: any) {
			overridesError = e.message;
		}
	}

	function deleteGroup(groupId: string) {
		deleteGroupId = groupId;
		deleteGroupOpen = true;
	}

	async function doDeleteGroup() {
		try {
			await api.del(`/v1/availability-overrides/group/${deleteGroupId}`);
			await loadOverrides();
		} catch (e: any) {
			overridesError = e.message;
		}
	}

	onMount(() => {
		loadOverrides();
	});

	const REASON_OPTIONS: { value: OverrideReason; label: string }[] = [
		{ value: 'day_off', label: 'Day off' },
		{ value: 'out_of_office', label: 'Out of office' },
		{ value: 'custom_hours', label: 'Custom hours' },
	];

	// Time-of-day options for the start/end selects, formatted per the user's prefs.
	let timeOptions = $derived(TIME_SLOTS.map((t) => ({ value: t, label: fmtTime(t) })));
	function timeLabel(t: string) { return fmtTime(t); }
</script>

<ConfirmDialog
	bind:open={deleteOvOpen}
	title="Remove date override?"
	description="Your default weekly hours will apply to this date again."
	confirmText="Remove"
	destructive
	onConfirm={doDeleteOverride}
/>

<ConfirmDialog
	bind:open={deleteGroupOpen}
	title="Remove this out-of-office span?"
	description="All days in this range will become bookable again."
	confirmText="Remove"
	destructive
	onConfirm={doDeleteGroup}
/>

<svelte:head><title>Availability — Calnode</title></svelte:head>

<div class="mb-8">
	<h1 class="text-2xl font-semibold tracking-tight">Availability</h1>
	<p class="mt-1 text-sm text-muted-foreground">Set your weekly hours and block off specific dates.</p>
</div>

<!-- Weekly Hours -->
<div class="mb-8">
	<h2 class="mb-3 text-sm font-semibold uppercase tracking-wider text-muted-foreground">Weekly Hours</h2>

	<WeeklyHours />
</div>

<!-- Date Overrides -->
<div>
	<h2 class="mb-1 text-sm font-semibold uppercase tracking-wider text-muted-foreground">Date Overrides</h2>
	<p class="mb-3 text-sm text-muted-foreground">Block out a specific date, or set custom hours for it.</p>

	<div class="rounded-lg border bg-card">
		{#if overridesError}<p class="px-4 pt-4 text-sm text-destructive">{overridesError}</p>{/if}
		{#if overlappingOverrideDates.length > 0}
			<p class="px-4 pt-4 text-xs text-amber-700">Overlapping blocks on {overlappingOverrideDates.map((d) => fmtDate(d, $prefs)).join(', ')} merge into one window — trim them if that wasn't the intent.</p>
		{/if}

		{#if overridesLoading}
			<p class="px-4 py-4 text-sm text-muted-foreground">Loading…</p>
		{:else if overrides.length > 0}
			<table class="w-full text-sm">
				<thead>
					<tr class="border-b">
						<th class="px-4 pb-3 pt-3 text-left text-xs font-medium text-muted-foreground">Date</th>
						<th class="px-4 pb-3 pt-3 text-left text-xs font-medium text-muted-foreground">Type</th>
						<th class="px-4 pb-3 pt-3 text-left text-xs font-medium text-muted-foreground">Duration</th>
						<th class="px-4 pb-3 pt-3"></th>
					</tr>
				</thead>
				<tbody class="divide-y">
					{#each overrideEntries as entry (entry.kind === 'span' ? entry.group_id : entry.ov.id)}
						{#if entry.kind === 'span'}
							<tr class="transition-colors hover:bg-muted/30">
								<td class="px-4 py-3 font-medium">{fmtDate(entry.start, $prefs)} – {fmtDate(entry.end, $prefs)}</td>
								<td class="px-4 py-3">
									{#if entry.reason === 'out_of_office'}
										<Badge class="bg-amber-50 text-amber-700 border-amber-200">Out of office</Badge>
									{:else}
										<Badge variant="secondary">Day off</Badge>
									{/if}
								</td>
								<td class="px-4 py-3 text-muted-foreground">{entry.days} days</td>
								<td class="px-4 py-3">
									<Tooltip.Provider>
										<div class="flex items-center justify-end gap-1">
											<Tooltip.Root>
												<Tooltip.Trigger class={buttonVariants({ variant: 'ghost', size: 'icon' })} onclick={() => deleteGroup(entry.group_id)}>
													<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/><path d="M9 6V4h6v2"/></svg>
												</Tooltip.Trigger>
												<Tooltip.Content>Delete span</Tooltip.Content>
											</Tooltip.Root>
										</div>
									</Tooltip.Provider>
								</td>
							</tr>
						{:else}
							{@const ov = entry.ov}
							<tr class="transition-colors hover:bg-muted/30">
								<td class="px-4 py-3 font-medium">{fmtDate(ov.date, $prefs)}</td>
								<td class="px-4 py-3">
									{#if ov.reason === 'custom_hours'}
										<Badge class="bg-blue-50 text-blue-700 border-blue-200">Custom hours</Badge>
									{:else if ov.reason === 'out_of_office'}
										<Badge class="bg-amber-50 text-amber-700 border-amber-200">Out of office</Badge>
									{:else}
										<Badge variant="secondary">Day off</Badge>
									{/if}
								</td>
								<td class="px-4 py-3 text-muted-foreground">
									{ov.is_available && ov.start_time && ov.end_time
										? `${fmtTime(ov.start_time)} – ${fmtTime(ov.end_time)}`
										: '1 day'}
								</td>
								<td class="px-4 py-3">
									<Tooltip.Provider>
										<div class="flex items-center justify-end gap-1">
											<Tooltip.Root>
												<Tooltip.Trigger
													class={buttonVariants({ variant: 'ghost', size: 'icon' })}
													onclick={() => deleteOverride(ov.id)}
												>
													<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/><path d="M10 11v6"/><path d="M14 11v6"/><path d="M9 6V4h6v2"/></svg>
												</Tooltip.Trigger>
												<Tooltip.Content>Delete</Tooltip.Content>
											</Tooltip.Root>
										</div>
									</Tooltip.Provider>
								</td>
							</tr>
						{/if}
					{/each}
				</tbody>
			</table>
		{:else}
			<p class="px-4 py-4 text-sm text-muted-foreground">No overrides yet.</p>
		{/if}

		<!-- Add override form -->
		<div class="border-t px-4 py-4">
			{#if ovAddError}<p class="mb-3 rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">{ovAddError}</p>{/if}
			<div class="flex flex-wrap items-end gap-3">
				<div class="space-y-1.5">
					<label for="ov-date" class="text-sm font-medium">Date</label>
					<DatePicker bind:value={ovForm.date} placeholder="Pick a date" />
				</div>
				{#if ovForm.reason !== 'custom_hours'}
					<div class="space-y-1.5">
						<label for="ov-end-date" class="text-sm font-medium">To <span class="font-normal text-muted-foreground">(optional)</span></label>
						<DatePicker bind:value={ovForm.end_date} placeholder="Same day" />
					</div>
				{/if}
				<div class="space-y-1.5">
					<label for="ov-type" class="text-sm font-medium">Type</label>
					<Select.Root type="single" value={ovForm.reason} onValueChange={(v) => { if (v) ovForm.reason = v as OverrideReason; }}>
						<Select.Trigger id="ov-type" class="w-fit">{REASON_LABELS[ovForm.reason]}</Select.Trigger>
						<Select.Content>
							{#each REASON_OPTIONS as r}
								<Select.Item value={r.value} label={r.label}>{r.label}</Select.Item>
							{/each}
						</Select.Content>
					</Select.Root>
				</div>
				{#if ovForm.reason === 'custom_hours'}
					<div class="space-y-1.5">
						<label for="ov-start" class="text-sm font-medium">From</label>
						<Select.Root type="single" bind:value={ovForm.start_time}>
							<Select.Trigger id="ov-start" class="w-fit">{timeLabel(ovForm.start_time)}</Select.Trigger>
							<Select.Content>
								{#each timeOptions as t}<Select.Item value={t.value} label={t.label}>{t.label}</Select.Item>{/each}
							</Select.Content>
						</Select.Root>
					</div>
					<div class="space-y-1.5">
						<label for="ov-end" class="text-sm font-medium">To</label>
						<Select.Root type="single" bind:value={ovForm.end_time}>
							<Select.Trigger id="ov-end" class="w-fit">{timeLabel(ovForm.end_time)}</Select.Trigger>
							<Select.Content>
								{#each timeOptions as t}<Select.Item value={t.value} label={t.label}>{t.label}</Select.Item>{/each}
							</Select.Content>
						</Select.Root>
					</div>
				{/if}
				<Button onclick={addOverride} disabled={addingOv}>
					{addingOv ? 'Adding…' : 'Add override'}
				</Button>
				{#if ovForm.reason === 'custom_hours'}
					<p class="w-full text-xs text-muted-foreground">Split day? Add one block, then add another on the same date.</p>
				{/if}
			</div>
		</div>
	</div>
</div>
