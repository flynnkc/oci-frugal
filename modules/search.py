#!/bin/python3.11

import logging

from modules.config import Config
from oci import pagination
from oci.resource_search import ResourceSearchClient
from oci.resource_search.models import StructuredSearchDetails
from oci.resource_search.models import ResourceSummaryCollection

class Search:

    search_types = [
    "instance",
    "volume",
    "vcn",
    "analyticsinstance",
    "apigateway",
    "bastion",
    "bootvolume",
    "certificateauthority",
    "instancepool",
    "clusterscluster",
    "clustersvirtualnode",
    "containerinstance",
    "datacatalog",
    "application",
    "disworkspace",
    "datascienceproject",
    "autonomousdatabase",
    "cloudexadatainfrastructure",
    "dbsystem",
    "devopsproject",
    "filesystem",
    "functionsfunction",
    "integrationinstance",
    "loadbalancer",
    "drg",
    "networkfirewall",
    "bucket",
    "stream",
    "vault",
    "vbsinstance"
    ]

    def __init__(self, config: Config):
        self.log = logging.getLogger(f'{__name__}.Search')
        self.log.info(f'Initializing Search Object {self}')
        self.config = config.config
        self.signer = config.signer
        self.tag = config.args.tag
        self.compartment = config.args.compartment
        self.exclude = config.args.exclude


    def query(self, region: str) -> ResourceSummaryCollection.items:
        self.client = ResourceSearchClient(self.config, signer=self.signer)
        self.log.debug(f'Initialized Search client: {self.client}')

        # Create structured search with parameters
        comma = ", "
        query = (f"query {comma.join(self.search_types)} resources where"
                 f" (definedTags.namespace = '{self.tag}')")
        query += f" && compartmentId  = '{self.compartment}'" if \
            self.compartment else ""
        query += f" && compartmentId  != '{self.exclude}'" if \
            self.exclude else ""

        details = StructuredSearchDetails(query=query)
        response = pagination.list_call_get_all_results(self.client.search_resources, details)
        self.log.debug(f'Response Headers: {response.headers}\n\t'
                       f'Request ID: {response.request_id}\n\tRequest: '
                       f'{response.request}\n\tStatus: {response.status}')
        self.log.debug(f'Search Response {response.data.items}')
        self.log.info(f'Found {len(response.data.items)} Resources')
        
        return response.data.items