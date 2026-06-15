// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// https://astro.build/config
export default defineConfig({
	site: 'https://MichaelSp.github.io',
	base: '/kswitch',
	integrations: [
		starlight({
			title: 'kswitch',
			description: 'The kubectx for operators — single pane of glass for all your kubeconfig files.',
			logo: {
				src: './src/assets/logo.png',
				replacesTitle: false,
			},
			favicon: '/favicon.png',
			social: [
				{ icon: 'github', label: 'GitHub', href: 'https://github.com/MichaelSp/kswitch' },
			],
			editLink: {
				baseUrl: 'https://github.com/MichaelSp/kswitch/edit/main/docs/',
			},
			customCss: ['./src/styles/custom.css'],
			sidebar: [
				{
					label: 'Getting Started',
					items: [
						{ label: 'Introduction', slug: 'index' },
						{ label: 'Installation', slug: 'installation' },
						{ label: 'How it works', slug: 'how_it_works' },
					],
				},
				{
					label: 'Usage',
					items: [
						{ label: 'Kubeconfig stores', slug: 'kubeconfig_stores' },
						{ label: 'Search index', slug: 'search_index' },
						{ label: 'Kubeconfig cache', slug: 'kubeconfig_cache' },
						{ label: 'Command completion', slug: 'command_completion' },
					],
				},
				{
					label: 'Stores',
					items: [
						{ label: 'Filesystem', slug: 'stores/filesystem/filesystem' },
						{ label: 'Vault', slug: 'stores/vault/use_vault_store' },
						{ label: 'Gardener', slug: 'stores/gardener/gardener' },
						{ label: 'Amazon EKS', slug: 'stores/eks/eks' },
						{ label: 'Azure AKS', slug: 'stores/azure/azure' },
						{ label: 'Google GKE', slug: 'stores/gke/gke' },
						{ label: 'DigitalOcean', slug: 'stores/digitalocean/digitalocean' },
						{ label: 'Rancher', slug: 'stores/rancher/rancher' },
						{ label: 'Exoscale', slug: 'stores/exoscale/exoscale' },
						{ label: 'OVH', slug: 'stores/ovh/ovh' },
						{ label: 'Akamai / Linode', slug: 'stores/akamai/akamai' },
						{ label: 'Cluster API', slug: 'stores/capi/capi' },
						{ label: 'Adding a store', slug: 'stores/adding-a-store' },
					],
				},
				{
					label: 'Advanced',
					items: [
						{ label: 'Hooks', slug: 'hooks' },
					],
				},
			],
		}),
	],
});
