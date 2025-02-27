// Copyright (c) Zefchain Labs, Inc.
// SPDX-License-Identifier: Apache-2.0

#![cfg_attr(target_arch = "wasm32", no_main)]

mod state;

use std::{
    collections::{BTreeMap, BTreeSet},
    sync::{Arc, Mutex},
};

use async_graphql::{EmptySubscription, Object, Request, Response, Schema};
use base64::engine::{general_purpose::STANDARD_NO_PAD, Engine as _};
use fungible::Account;
use linera_sdk::{
    base::{AccountOwner, WithServiceAbi},
    views::View,
    DataBlobHash, Service, ServiceRuntime,
};
use non_fungible::{NftOutput, Operation, TokenId};

use self::state::NonFungibleTokenState;

pub struct NonFungibleTokenService {
    state: Arc<NonFungibleTokenState>,
    runtime: Arc<Mutex<ServiceRuntime<Self>>>,
}

linera_sdk::service!(NonFungibleTokenService);

impl WithServiceAbi for NonFungibleTokenService {
    type Abi = non_fungible::NonFungibleTokenAbi;
}

impl Service for NonFungibleTokenService {
    type Parameters = ();

    async fn new(runtime: ServiceRuntime<Self>) -> Self {
        let state = NonFungibleTokenState::load(runtime.root_view_storage_context())
            .await
            .expect("Failed to load state");
        NonFungibleTokenService {
            state: Arc::new(state),
            runtime: Arc::new(Mutex::new(runtime)),
        }
    }

    async fn handle_query(&self, request: Request) -> Response {
        let schema = Schema::build(
            QueryRoot {
                non_fungible_token: self.state.clone(),
                runtime: self.runtime.clone(),
            },
            MutationRoot,
            EmptySubscription,
        )
        .finish();
        schema.execute(request).await
    }
}

struct QueryRoot {
    non_fungible_token: Arc<NonFungibleTokenState>,
    runtime: Arc<Mutex<ServiceRuntime<NonFungibleTokenService>>>,
}

#[Object]
impl QueryRoot {
    async fn nft(&self, token_id: String) -> Option<NftOutput> {
        let token_id_vec = STANDARD_NO_PAD.decode(&token_id).unwrap();
        let nft = self
            .non_fungible_token
            .nfts
            .get(&TokenId { id: token_id_vec })
            .await
            .unwrap();

        if let Some(nft) = nft {
            let payload = {
                let mut runtime = self
                    .runtime
                    .try_lock()
                    .expect("Services only run in a single thread");
                runtime.read_data_blob(nft.blob_hash)
            };
            let nft_output = NftOutput::new_with_token_id(token_id, nft, payload);
            Some(nft_output)
        } else {
            None
        }
    }

    async fn nftUsingBlobHash(&self, id: u64) -> Option<NftOutput> {
        let token_id = self.non_fungible_token.blob_token_ids.get(&id).await.unwrap();

        let nft = self
            .non_fungible_token
            .nfts
            .get(&token_id.clone().unwrap())
            .await
            .unwrap();

        if let Some(nft) = nft {
            let payload = {
                let mut runtime = self
                    .runtime
                    .try_lock()
                    .expect("Services only run in a single thread");
                runtime.read_data_blob(nft.blob_hash)
            };
            let nft_output = NftOutput::new_with_token_id(token_id.unwrap().to_string(), nft, payload);
            Some(nft_output)
        } else {
            None
        }
    }

    async fn nfts(&self) -> BTreeMap<String, NftOutput> {
        let mut nfts = BTreeMap::new();
        self.non_fungible_token
            .nfts
            .for_each_index_value(|_token_id, nft| {
                let nft = nft.into_owned();
                let payload = {
                    let mut runtime = self
                        .runtime
                        .try_lock()
                        .expect("Services only run in a single thread");
                    runtime.read_data_blob(nft.blob_hash)
                };
                let nft_output = NftOutput::new(nft, payload);
                nfts.insert(nft_output.token_id.clone(), nft_output);
                Ok(())
            })
            .await
            .unwrap();

        nfts
    }

    async fn owned_token_ids_by_owner(&self, owner: AccountOwner) -> BTreeSet<String> {
        self.non_fungible_token
            .owned_token_ids
            .get(&owner)
            .await
            .unwrap()
            .into_iter()
            .flatten()
            .map(|token_id| STANDARD_NO_PAD.encode(token_id.id))
            .collect()
    }

    async fn owned_token_ids(&self) -> BTreeMap<AccountOwner, BTreeSet<String>> {
        let mut owners = BTreeMap::new();
        self.non_fungible_token
            .owned_token_ids
            .for_each_index_value(|owner, token_ids| {
                let token_ids = token_ids.into_owned();
                let new_token_ids = token_ids
                    .into_iter()
                    .map(|token_id| STANDARD_NO_PAD.encode(token_id.id))
                    .collect();

                owners.insert(owner, new_token_ids);
                Ok(())
            })
            .await
            .unwrap();

        owners
    }

    async fn owned_nfts(&self, owner: AccountOwner) -> BTreeMap<String, NftOutput> {
        let mut result = BTreeMap::new();
        let owned_token_ids = self
            .non_fungible_token
            .owned_token_ids
            .get(&owner)
            .await
            .unwrap();

        for token_id in owned_token_ids.into_iter().flatten() {
            let nft = self
                .non_fungible_token
                .nfts
                .get(&token_id)
                .await
                .unwrap()
                .unwrap();
            let payload = {
                let mut runtime = self
                    .runtime
                    .try_lock()
                    .expect("Services only run in a single thread");
                runtime.read_data_blob(nft.blob_hash)
            };
            let nft_output = NftOutput::new(nft, payload);
            result.insert(nft_output.token_id.clone(), nft_output);
        }

        result
    }
}

struct MutationRoot;

#[Object]
impl MutationRoot {
    async fn mint(&self, minter: AccountOwner, name: String, blob_hash: DataBlobHash,
                  token: String, // ETH, SOL
                  price: String, // 0.05 [token]
                  id: u64, // specific chain nft id
                  chain_minter: String, // chain nft minter
                  chain_owner: String, // chain nft owner
                  description: String,
                  ) -> Vec<u8> {
        bcs::to_bytes(&Operation::Mint {
            minter,
            name,
            blob_hash,
            token,
            price,
            id,
            chain_owner,
            chain_minter,
            description,
        })
        .unwrap()
    }

    async fn transfer(
        &self,
        source_owner: AccountOwner,
        token_id: String,
        target_account: Account,
        chain_owner: String,
        buy_from_token: String,
        to_token: String,
        amount: String,
    ) -> Vec<u8> {
        bcs::to_bytes(&Operation::Transfer {
            source_owner,
            token_id: TokenId {
                id: STANDARD_NO_PAD.decode(token_id).unwrap(),
            },
            target_account,
            chain_owner,
            buy_from_token,
            to_token,
            amount,
        })
        .unwrap()
    }

    async fn claim(
        &self,
        source_account: Account,
        token_id: String,
        target_account: Account,
    ) -> Vec<u8> {
        bcs::to_bytes(&Operation::Claim {
            source_account,
            token_id: TokenId {
                id: STANDARD_NO_PAD.decode(token_id).unwrap(),
            },
            target_account,
        })
        .unwrap()
    }

    async fn listNftForSale(
        &self,
        token_id: String,
        chain_owner: String,
    ) -> Vec<u8> {
        bcs::to_bytes(&Operation::ListNftForSale {
            token_id: TokenId {
                id: STANDARD_NO_PAD.decode(token_id).unwrap(),
            },
            chain_owner,
        }).unwrap()
    }
}
