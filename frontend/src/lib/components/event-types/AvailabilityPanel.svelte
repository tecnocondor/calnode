<script lang="ts">
	// Hours for one event type. Off: the event follows the host's weekly hours
	// (Availability page). On: it follows only its own rules, so it can offer fewer
	// hours than the rest of the calendar (e.g. a training booked only in the afternoon).
	// Date overrides (days off, custom hours) still apply on top, as for every event.
	import { onMount } from 'svelte';
	import { api, type AvailabilityRule } from '$lib/api';
	import { Switch } from '$lib/components/ui/switch';
	import { Label } from '$lib/components/ui/label';
	import { ConfirmDialog } from '$lib/components/ui/confirm-dialog';
	import { toast } from 'svelte-sonner';
	import WeeklyHours from '$lib/components/availability/WeeklyHours.svelte';

	let { eventTypeId }: { eventTypeId: string } = $props();

	let loading = $state(true);
	let custom = $state(false);
	let busy = $state(false);
	let confirmOffOpen = $state(false);

	const rulesFor = (id?: string) =>
		api.get<{ items: AvailabilityRule[] }>(
			id ? `/v1/availability-rules?event_type_id=${encodeURIComponent(id)}` : '/v1/availability-rules'
		);

	async function load() {
		loading = true;
		try {
			const res = await rulesFor(eventTypeId);
			custom = (res.items ?? []).length > 0;
		} catch (e: any) {
			toast.error(e.message || 'Could not load the hours');
		} finally {
			loading = false;
		}
	}

	// Turning it on starts from a copy of the weekly hours, so the event keeps
	// offering the same times until someone narrows them.
	async function turnOn() {
		busy = true;
		try {
			const global = await rulesFor();
			// The weekly hours can hold the same block twice (event_type_id NULL is never
			// equal in the unique index); copying it twice would 409 and stop halfway.
			const seen = new Set<string>();
			for (const r of global.items ?? []) {
				const k = `${r.day_of_week}|${r.start_time}|${r.end_time}`;
				if (seen.has(k)) continue;
				seen.add(k);
				await api.post('/v1/availability-rules', {
					event_type_id: eventTypeId,
					day_of_week: r.day_of_week,
					start_time: r.start_time,
					end_time: r.end_time
				});
			}
			custom = true;
		} catch (e: any) {
			toast.error(e.message || 'Could not copy the weekly hours');
			await load();
		} finally {
			busy = false;
		}
	}

	async function turnOff() {
		busy = true;
		try {
			const own = await rulesFor(eventTypeId);
			for (const r of own.items ?? []) await api.del(`/v1/availability-rules/${r.id}`);
			custom = false;
		} catch (e: any) {
			toast.error(e.message || 'Could not remove the custom hours');
			await load();
		} finally {
			busy = false;
		}
	}

	function onToggle(v: boolean) {
		if (v) turnOn();
		else {
			custom = true; // stays on until the dialog is confirmed
			confirmOffOpen = true;
		}
	}

	onMount(load);
</script>

<ConfirmDialog
	bind:open={confirmOffOpen}
	title="Use your weekly hours again?"
	description="The custom hours of this event are removed, and it goes back to offering the hours set on the Availability page."
	confirmText="Use weekly hours"
	destructive
	onConfirm={turnOff}
/>

<div class="mb-6 rounded-lg border bg-card p-4">
	<div class="flex items-start justify-between gap-4">
		<div>
			<Label for="custom-hours" class="text-sm font-medium">Custom hours for this event</Label>
			<p class="mt-1 text-sm text-muted-foreground">
				{#if custom}
					This event is bookable only during the hours below, even if your weekly hours are wider. Date overrides still apply. Removing every block brings back your weekly hours.
				{:else}
					This event follows your weekly hours from the Availability page. Turn this on to offer it at fewer, or different, times.
				{/if}
			</p>
		</div>
		<Switch id="custom-hours" bind:checked={custom} disabled={loading || busy} onCheckedChange={onToggle} />
	</div>
</div>

{#if !loading && custom && !busy}
	<WeeklyHours {eventTypeId} />
{/if}
